// Copyright 2015 James Cote and Liberty Fund, Inc.
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package batch_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/jfcote87/google-api-go-client/batch"

	"io/ioutil"

	"net/http/httptest"

	"google.golang.org/api/googleapi"
)

var testResponse = `HTTP/1.1 200 OK
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

type TestStruct struct {
	A string
	B int
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
func TestResponseParse(t *testing.T) {
	// Sanity check: pass in **ptr and receive *ptr
	var res batch.Response
	req := &batch.Request{}
	var rd io.Reader
	//Zero Request should create a blank response
	res = batch.NewResponse(rd, req)
	if res.Err != nil {
		t.Errorf("Expected no error on empty reader; got: %v", res.Err)
	}
	// nil reader with resultPtr should provide error "No JSon data returned"
	// and set result to a nil valued ptr of proper type
	var testPtr *TestStruct
	batch.SetResult(&testPtr)(req)
	res = batch.NewResponse(rd, req)
	if tval, ok := res.Result.(*TestStruct); !ok || tval != nil {
		t.Errorf("Wanted nil ptr value when response bytes are empty; got %v", reflect.ValueOf(testPtr).Type())
	}
	if res.Err == nil || res.Err.Error() != "No JSON data returned" {
		t.Errorf("Wanted \"No JSON data returned\" on empty response; got %v", res.Err)
	}

	// reader with json error
	buf := bytes.NewBufferString("{ \"A\": 5, \"B\":\"TesTestStructing\"}")
	res = batch.NewResponse(rd, req)
	if res.Err == nil {
		t.Errorf("Wanted invalid json error; got nil")
	}

	// good reader should return nil Err and a Result of *TestStruct
	buf = bytes.NewBufferString("{ \"B\": 5, \"A\":\"TesTestStructing\"}")
	res = batch.NewResponse(buf, req)
	if res.Err != nil {
		t.Fatalf("Wanted no error; got: %v", res.Err)
	}
	if tval, ok := res.Result.(*TestStruct); !ok || tval == nil || tval.B != 5 || tval.A != "TesTestStructing" {
		t.Errorf(`Wanted TestStruct{A: "TesTestStructing", B: 5}; got %#v`, tval)
	}
}

// TestBodyProcess
func TestBodyProcess(t *testing.T) {
	tb := bytes.NewBuffer([]byte(strings.Replace(testResponse, "\n", "\r\n", -1)))
	res, err := http.ReadResponse(bufio.NewReader(tb), nil)
	if err != nil {
		t.Fatalf("Wanted test response to create http.Response; got %v", err)
	}

	var t1 *ObjectAccessControl
	var t2 *ObjectAccessControl
	var t3 *ObjectAccessControl

	rq := make([]*batch.Request, 4)
	rq[0] = &batch.Request{}
	rq[1] = &batch.Request{}
	rq[2] = &batch.Request{}
	rq[3] = &batch.Request{}
	batch.SetResult(&t1)(rq[0])
	batch.SetTag("T1")(rq[0])
	batch.SetResult(&t2)(rq[1])
	batch.SetTag("T2")(rq[1])
	batch.SetResult(&t3)(rq[2])
	batch.SetTag("T3")(rq[2])
	batch.SetTag("T4")(rq[3])

	cType, params, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf(("Wanted testResponse to parse correctly; got %v"), err)
	}
	if !strings.HasPrefix(cType, "multipart/") {
		t.Errorf("Wanted ParseMediaType to return multipart type; got %s", cType)
		return
	}
	_ = params

	results, err := batch.ProcessBody(context.Background(), res.Body, params["boundary"], rq)
	if err != nil || len(results) != len(rq) {
		t.Fatalf("Wanted ProcessBody to return %d results; got %d results with error %v", len(rq), len(results), err)
	}

	rx := results[0]
	if _, ok := rx.Err.(*googleapi.Error); !ok {
		t.Errorf("Wanted first batch item to return google api error; go %#v", err)
	}
	if strData, ok := rx.Tag.(string); !ok || strData != "T1" {
		t.Errorf("Wanted first batch item tagged as \"T1\"; got %v", rx.Tag)
	}

	rx = results[1]
	if rx.Err == nil || !strings.HasPrefix(rx.Err.Error(), "invalid character 'i'") {
		t.Errorf("Wanted 2nd batch item to return json parse error; got %v", rx.Err)
	}
	if strData, ok := rx.Tag.(string); !ok || strData != "T2" {
		t.Errorf("Wanted 2nd batch item tagged as \"T2\"; got %v", rx.Tag)
	}

	rx = results[2]
	if rx.Err != nil || rx.Result == nil || rx.Result.(*ObjectAccessControl).Object != "obj3" {
		t.Errorf("Wanted 3rd batch to return ObjectAccessControl struct; got %#v %v", rx.Result, rx.Err)
	}
	if strData, ok := rx.Tag.(string); !ok || strData != "T3" {
		t.Errorf("Wanted 3rd batch item tagged as \"T3\"; got %v", rx.Tag)
	}

	rx = results[3]
	if rx.Err != nil || rx.Result != nil {
		t.Errorf("Wanted 4th batch item to return nil ptr and nil error; got %v  %v", rx.Result, rx.Err)
	}
	if strData, ok := rx.Tag.(string); !ok || strData != "T4" {
		t.Errorf("Wanted 4th batch item tagged as \"T4\"; got %v", rx.Tag)
	}
	return
}

func TestAddRequest(t *testing.T) {
	err := fmt.Errorf("This is not a batch.Request")
	sv := &batch.Service{}
	if err = sv.AddRequest(err); err == nil {
		t.Errorf("Wanted AddRequest to fail; got %v", err)
	}

	b := &batch.Request{}
	var tx *TestStruct
	if err = batch.SetResult(tx)(b); err == nil {
		t.Errorf("Wanted error Invalid Result Pointer Value; got nil error")
	}
	batch.SetTag("TAG")(b)
	if err = batch.SetResult(&tx)(b); err != nil {
		t.Errorf("Wanted good SetResult; got %v", err)
	} else {
		newResp := batch.NewResponse(nil, b)
		if testPtr, ok := newResp.Result.(*TestStruct); !ok || testPtr != tx {
			t.Errorf("Wantd %#v; got %#v", tx, testPtr)
		}
		newResp = batch.NewResponse(bytes.NewBufferString("{\"A\":\"A\",\"B\":9}"), b)
		if testPtr, ok := newResp.Result.(*TestStruct); !ok || testPtr == nil || testPtr.A != "A" || testPtr.B != 9 {
			t.Errorf("SetResult: Expected testPtr to equal {A:\"A\", B:9}; got %#v", testPtr)
		}
	}
	sv.Client = &http.Client{Transport: &TestTransport{}}
	sv.MaxRequests = 2

	req, _ := http.NewRequest("POST", "http://example.com/upload/example/uri", bytes.NewBuffer([]byte("PAYLOAD")))
	req.Header.Set("host", "testHost")
	req.Header.Set("User-Agent", "TestUA")
	_, d := batch.BatchClient.Do(req)
	if d == nil || !strings.HasSuffix(d.Error(), "Media Uploads not allowed for a BatchItem") {
		t.Errorf("Expected \"BatchApi: Media Uploads not allowed for a BatchItem\"; got %v", d)
	}

	req, _ = http.NewRequest("GET", "http://example.com/example/uri", bytes.NewBuffer([]byte("PAYLOAD")))
	req.Header.Set("host", "testHost")
	req.Header.Set("User-Agent", "TestUA")
	_, d = batch.BatchClient.Do(req)
	err = sv.AddRequest(d, batch.SetResult(&tx), batch.SetTag("1st Request"))
	if err != nil {
		t.Errorf("Wanted SetTag success; got %v", err)
	}
	err = sv.AddRequest(d, batch.SetTag("2nd Request"))
	err = sv.AddRequest(d, batch.SetResult(&tx), batch.SetTag("3rd Request"), batch.SetCredentials(TestCredential{}))

	if sv.Count() != 3 {
		t.Errorf("Wanted count to be 3 instead; got %d", sv.Count())
		return
	}

	_, err = sv.Do()
	// make certain we went thru ErrorTransport
	err, ok := err.(*url.Error)
	if !ok || err.(*url.Error).Err.Error() != "Test Transport" {
		t.Errorf("Wanted \"TestTransport error\"; got %v", err)
	}

	// Ensure that on MaxRequests were sent and that remaining were still stored
	if sv.Count() != 1 {
		t.Errorf("Expected count to be 1; got %d", sv.Count())
	}
	sv.Client = &http.Client{Transport: &TestTransport2{}}
	results, err := sv.Do()
	if err != nil || len(results) != 1 {
		t.Fatalf("Wanted 1 Response; got %d responses - %v", len(results), err)
	}
	if results[0].Tag != "3rd Request" || tx.A != "STRING" || tx.B != 9 {
		t.Errorf("Wanted 3rd Response with value of {\"A\":\"STRING\", \"B\":9}; got %s - %v", results[0].Tag, tx)
	}
}

type TestTransport struct{}

func (tr *TestTransport) RoundTrip(req *http.Request) (res *http.Response, err error) {
	b := bytes.NewBuffer(make([]byte, 0, req.ContentLength))
	io.Copy(b, req.Body)
	req.Body.Close()
	return nil, fmt.Errorf("Test Transport")
}

type TestTransport2 struct{}

func (tr *TestTransport2) RoundTrip(req *http.Request) (*http.Response, error) {
	_, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	defer req.Body.Close()
	mr := multipart.NewReader(req.Body, params["boundary"])
	pr, err := mr.NextPart()
	if err != nil {
		return nil, err
	}
	defer pr.Close()
	if pr.Header.Get("Content-Id") != "batch0000" || pr.Header.Get("Content-Type") != "application/http" {
		return nil, fmt.Errorf("Expected Content-Id = batch0000 and Content-Type = application/http; got %s, %s",
			pr.Header.Get("Content-Id"), pr.Header.Get("Content-Type"))
	}
	rx, err := http.ReadRequest(bufio.NewReader(pr))
	if err != nil {
		return nil, err
	}
	defer rx.Body.Close()
	if rx.Header.Get("Authorization") != "AuthString" {
		return nil, fmt.Errorf("Expected Authorization header = AuthString; got %s", rx.Header.Get("Authorization"))
	}
	bx, err := ioutil.ReadAll(rx.Body)
	if err != nil {
		return nil, err
	}
	if string(bx) != "PAYLOAD" {
		return nil, fmt.Errorf("Expected PAYLOAD; got %s", string(bx))
	}
	//return nil, errors.New("My Error")

	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "multipart/mixed; boundary=batchXpK7JBAk73XEXXAA5eFwv4m2QX")
	rec.Header().Set("Date", "Tue, 22 Jan 2013 18:56:00 GMT")
	rec.WriteString("--batchXpK7JBAk73XEXXAA5eFwv4m2QX\r\n")
	rec.WriteString("Content-Type: application/http\r\n")
	rec.WriteString("Content-ID: <response-8a09ca85-8d1d-4f45-9eb0-da8e8b07ec83+1>\r\n\r\n")
	rec.WriteString("HTTP/1.1 200 OK\r\n")
	rec.WriteString("Content-Type: application/json; charset=UTF-8\r\n")
	rec.WriteString("Content-Length: 22\r\n\r\n{\"A\":\"STRING\",\"B\":9}\r\n")
	rec.WriteString("--batchXpK7JBAk73XEXXAA5eFwv4m2QX--")

	res := rec.Result()

	return res, err
}

type TestCredential struct{}

func (tc TestCredential) Authorization() (string, error) {
	return "AuthString", nil
}
