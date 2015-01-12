// Copyright 2015 James Cote and Liberty Fund, Inc.
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package batch

import (
	"bufio"
	"bytes"
	"fmt"
	"google.golang.org/api/googleapi"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
)

var testResponse string = `HTTP/1.1 200 OK
Content-Type: multipart/mixed; boundary=batchXpK7JBAk73XEXXAA5eFwv4m2QX
Date: Tue, 22 Jan 2013 18:56:00 GMT
Expires: Tue, 22 Jan 2013 18:56:00 GMT
Cache-Control: private, max-age=0
Content-Length: 2101

--batchXpK7JBAk73XEXXAA5eFwv4m2QX
Content-Type: application/http
Content-ID: <response-8a09ca85-8d1d-4f45-9eb0-da8e8b07ec83+1>

HTTP/1.1 302 OK
ETag: "lGaP-E0memYDumK16YuUDM_6Gf0/V43j6azD55CPRGb9b6uytDYl61Y"
Content-Type: application/json; charset=UTF-8
Date: Tue, 22 Jan 2013 18:56:00 GMT
Expires: Tue, 22 Jan 2013 18:56:00 GMT
Cache-Control: private, max-age=0
Content-Length: 242

{"kind":"storage#objectAccessControl","id":"example-bucket/obj1/allUsers","selfLink":"https://www.googleapis.com/storage/v1/b/example-bucket/o/obj1/acl/allUsers","bucket":"example-bucket","object":"obj1","entity":"allUsers","role":"READER"}
--batchXpK7JBAk73XEXXAA5eFwv4m2QX
Content-Type: application/http
Content-ID: 

HTTP/1.1 200 OK
ETag: "lGaP-E0memYDumK16YuUDM_6Gf0/91POdd-sxSAkJnS8Dm7wMxBSDKk"
Content-Type: application/json; charset=UTF-8
Date: Tue, 22 Jan 2013 18:56:00 GMT
Expires: Tue, 22 Jan 2013 18:56:00 GMT
Cache-Control: private, max-age=0
Content-Length: 242

{"kind":"storage#objectAccessControl", id":"example-bucket/obj2/allUsers","selfLink":"https://www.googleapis.com/storage/v1/b/example-bucket/o/obj2/acl/allUsers","bucket":"example-bucket","object":"obj2","entity":"allUsers","role":"READER"}
--batchXpK7JBAk73XEXXAA5eFwv4m2QX
Content-Type: application/http
Content-ID: 

HTTP/1.1 200 OK
ETag: "lGaP-E0memYDumK16YuUDM_6Gf0/d2Z1F1_ZVbB1dC0YKM9rX5VAgIQ"
Content-Type: application/json; charset=UTF-8
Date: Tue, 22 Jan 2013 18:56:00 GMT
Expires: Tue, 22 Jan 2013 18:56:00 GMT
Cache-Control: private, max-age=0
Content-Length: 242

{"kind":"storage#objectAccessControl","id":"example-bucket/obj3/allUsers","selfLink":"https://www.googleapis.com/storage/v1/b/example-bucket/o/obj3/acl/allUsers","bucket":"example-bucket","object":"obj3","entity":"allUsers","role":"READER"}
--batchXpK7JBAk73XEXXAA5eFwv4m2QX
Content-Type: application/http
Content-ID: 123

HTTP/1.1 200 OK
ETag: "lGaP-E0memYDumK16YuUDM_6Gf0/d2Z1F1_ZVbB1dC0YKM9rX5VAgIQ"
Date: Tue, 22 Jan 2013 18:56:00 GMT
Expires: Tue, 22 Jan 2013 18:56:00 GMT


--batchXpK7JBAk73XEXXAA5eFwv4m2QX--
`

type Tstr struct {
	A string
	B int
}

type testError struct {
	E error
}

func (e testError) Error() string {
	return e.E.Error()
}

type ObjectAccessControl struct {
	Bucket     string `json:"bucket,omitempty"`
	Domain     string `json:"domain,omitempty"`
	Email      string `json:"email,omitempty"`
	Entity     string `json:"entity,omitempty"`
	EntityId   string `json:"entityId,omitempty"`
	Etag       string `json:"etag,omitempty"`
	Generation int64  `json:"generation,omitempty,string"`
	Id         string `json:"id,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Object     string `json:"object,omitempty"`
	//ProjectTeam *ObjectAccessControlProjectTeam `json:"projectTeam,omitempty"`
	Role     string `json:"role,omitempty"`
	SelfLink string `json:"selfLink,omitempty"`
}

//  JSON Response Tests
func TestJSONParse(t *testing.T) {
	log.SetOutput(os.Stdout)

	// Sanity check: pass in **ptr and receive *ptr
	res := &Response{}
	req := &Request{}
	var rd io.Reader
	//Zero Request should create a blank response
	req.processResponse(rd, res)
	if res.Err != nil {
		log.Fatalf("Zero Request Err: %v", res.Err)
	}
	// nil reader with resultPtr should provide error "No JSon data returned"
	// and set result to a nil valued ptr of proper type
	var xptr *Tstr
	req.resultPtr = &xptr
	req.processResponse(rd, res)
	if t, ok := res.Result.(*Tstr); !ok || t != nil {
		log.Fatalf("Nil Reader with result should return nil value ptr of %v", reflect.ValueOf(xptr).Type())
	}
	if res.Err == nil || res.Err.Error() != "No JSON data returned" {
		log.Fatalf("Nil Reader with result should return error of No Json data returned")
	}

	// reader with json error
	buf := bytes.NewBufferString("{ \"A\": 5, \"B\":\"Teststring\"}")
	req.processResponse(buf, res)
	if res.Err == nil {
		log.Fatalf("Reader with json error should return error ")
	}

	// good reader should return nil Err and a Result of *Tstr
	res.Err = nil
	res.Result = nil
	buf = bytes.NewBufferString("{ \"B\": 5, \"A\":\"Teststring\"}")
	req.processResponse(buf, res)
	if res.Err != nil {
		log.Fatalf("Good Read Error: %v", res.Err)
	}
	if t, ok := res.Result.(*Tstr); !ok {
		log.Fatalf("Good Read should return *Tstr instead is %v", reflect.ValueOf(reflect.ValueOf(res.Result).Type()))
	} else {
		if t.B != 5 && t.A != "Teststring" {
			log.Fatalf("Good Read Invalid Values %d %s", t.B, t.A)
		}
	}
}

// TestBodyProcess
func TestBodyProcess(t *testing.T) {
	tb := bytes.NewBuffer([]byte(strings.Replace(testResponse, "\n", "\r\n", -1)))
	res, err := http.ReadResponse(bufio.NewReader(tb), nil)
	if err != nil {
		t.Fatalf("RESPONSE load: %v", err)
		return
	}

	var t1 *ObjectAccessControl
	var t2 *ObjectAccessControl
	var t3 *ObjectAccessControl

	rq := make([]*Request, 4)
	rq[0] = &Request{resultPtr: &t1, tag: "T1"}
	rq[1] = &Request{resultPtr: &t2, tag: "T2"}
	rq[2] = &Request{resultPtr: &t3, tag: "T3"}
	rq[3] = &Request{tag: "T4"}

	cType, params, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if err != nil {
		t.Errorf(("Batch Api: %v\n"), err)
		return
	}
	if !strings.HasPrefix(cType, "multipart/") {
		t.Errorf("BatchApi: Invalid Content Type returned %s", cType)
		return
	}
	_ = params

	results, err := processBody(res.Body, params["boundary"], rq)
	if err != nil {
		t.Fatalf("Process Body Error: %v", err)
		return
	}

	if len(results) != len(rq) {
		t.Fatalf("Process Body Error: Number of results %d should equal number of requests %d", len(results), len(rq))
	}

	rx := results[0]
	if rx.Err == nil {
		t.Logf("Tag %s  Err %v", rx.Tag.(string), rx.Err)
		t.Error("First Batch Item should return a googleapi error")
	} else {
		if e, ok := rx.Err.(*googleapi.Error); !ok {
			t.Errorf("First Batch Item should return google api error:  received %v\n", e)
		}
		if strData, ok := rx.Tag.(string); !ok || strData != "T1" {
			t.Errorf("First Batch Item should have T1 as data: %v\n", rx.Tag)
		}
	}

	rx = results[1]
	//if rx.Err != nil || rx.Result == nil || rx.Result.(*ObjectAccessControl).Object != "obj2" {
	if rx.Err == nil || !strings.HasPrefix(rx.Err.Error(), "invalid character 'i'") {
		t.Errorf("2nd Batch Item should have json error: %v\n", rx.Err)
	}

	if strData, ok := rx.Tag.(string); !ok || strData != "T2" {
		t.Errorf("2nd Batch Item should have T2 as data: %v\n", rx.Tag)
	}

	rx = results[2]
	if rx.Err != nil || rx.Result == nil || rx.Result.(*ObjectAccessControl).Object != "obj3" {
		t.Errorf("3rd Batch Item should be error free and return object obj3: %v\n", rx.Err)
	}
	if strData, ok := rx.Tag.(string); !ok || strData != "T3" {
		t.Errorf("3rd Batch Item should have T3 as data: %v\n", rx.Tag)
	}

	rx = results[3]
	if rx.Err != nil || rx.Result != nil {
		t.Errorf("4th Batch Item should have no error and return nil: %v  %v\n", rx.Err, rx.Result)
	}
	if strData, ok := rx.Tag.(string); !ok || strData != "T4" {
		t.Errorf("4th Batch Item should have T4 as data: %v\n", rx.Tag)
	}

	return
}

func TestAddRequest(t *testing.T) {
	err := fmt.Errorf("This is not a batch.Request")
	sv := &Service{}
	if err = sv.AddRequest(err); err == nil {
		t.Errorf("Add Request should have failed with a non batch.Request error")
	}

	d := &requestData{}
	b := &Request{data: d}
	var tx *Tstr
	if err = SetResult(tx)(b); err == nil {
		t.Errorf("Set result to a *ptr should error.  Need a **ptr")
	}
	if err = SetResult(&tx)(b); err != nil {
		t.Errorf("SetResult Fail: %v", err)
	} else {
		if testPtr, ok := b.resultPtr.(**Tstr); !ok || testPtr != &tx {
			t.Errorf("SetResult Fail: Pointer not initialized properly")
		}
	}
	//t.Errorf("%v", err)
	sv.Client = &http.Client{Transport: &TestTransport{}}
	sv.MaxRequests = 2

	d.method = "GET"
	d.uri = "/example/uri"
	d.header = make(http.Header)
	d.header.Set("A", "B")
	d.header.Set("D", "F")
	d.body = []byte("THIS IS A BODY")
	b.status = requestStatusQueued

	err = sv.AddRequest(d, SetResult(&tx), SetTag("1st Request"))
	if err != nil {
		t.Errorf("AddRequst Failed: %v", err)
	}
	err = sv.AddRequest(d, SetTag("2nd Request"))
	err = sv.AddRequest(d, SetTag("3rd Request"))
	if len(sv.requests) != 3 {
		t.Errorf("AddRequest count should be 3 instead is %d", len(sv.requests))
		return
	}

	_, err = sv.Do()
	// make certain we went thru ErrorTransport
	err, ok := err.(*url.Error)
	if !ok || err.(*url.Error).Err.Error() != "Test Transport" {
		t.Errorf("AddRequest: TestTransport error %v", err)
	}

	// Ensure that on MaxRequests were sent and that remaining were still stored
	if len(sv.requests) != 1 {
		for i, rx := range sv.requests {

			log.Printf("Request %d: %v\n", i, rx.tag)
		}
		t.Errorf("AddRequest count should be 1 instead is %d", len(sv.requests))
	}
	//b = &request

}

type TestTransport struct{}

func (tr *TestTransport) RoundTrip(req *http.Request) (res *http.Response, err error) {
	b := bytes.NewBuffer(make([]byte, 0, req.ContentLength))
	io.Copy(b, req.Body)
	req.Body.Close()
	//log.Printf("%s\n", string(b.Bytes()))
	return nil, fmt.Errorf("Test Transport")
}
