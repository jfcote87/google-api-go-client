// Copyright 2015 James Cote and Liberty Fund, Inc.
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package batch implements a service to use the google client api
// batch protocol.  For specifics of the batch protocol see
// https://cloud.google.com/storage/docs/json_api/v1/how-tos/batch
package batch

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/net/context/ctxhttp"

	"io/ioutil"

	"google.golang.org/api/googleapi"
)

const baseURL = "https://www.googleapis.com/batch"

type requestStatus int8

const (
	requestStatusQueued requestStatus = iota
	requestStatusError
	requestStatusSuccess
)

var skipHeaders = map[string]bool{
	"host":       true,
	"User-Agent": true,
}

var logFlag bool

// Error used when looping through individual responses
func batchError(r *Request, e error) error {
	return fmt.Errorf("BatchApi: Request Write Error: (%v) %s", r.tag, e.Error())
}

// Credentialer provides oauth credentials to a request
type Credentialer interface {
	Authorization() (string, error)
	//SetAuthHeader(*http.Request) error
}

// requestData stores header, url and body information for a Request.  It implements
// the error interface so it may be passed back in the Do() client call
type requestData struct {
	method string
	uri    string
	header http.Header
	body   []byte
}

// Error implements error interface so requestData may be returned via the err var in client call
func (d *requestData) Error() string {
	return fmt.Sprintf("Batched Request: %s %s", d.method, d.uri)
}

// BatchClient is used to initiate a client api service.  All client api
// requests return a batchData struct.
var BatchClient = &http.Client{Transport: &batchIntercept{}}

type batchIntercept struct{}

// RoundTrip captures the client api call copying header, url and  body to a BatchItem
// for later processing.
func (bt *batchIntercept) RoundTrip(req *http.Request) (*http.Response, error) {
	// check for media upload as a batch request cannot contain a media upload
	if strings.HasPrefix(req.URL.Path, "/upload") {
		return nil, errors.New("BatchApi: Media Uploads not allowed for a BatchItem")
	}

	if s := req.Header.Get("Content-Type"); s != "" && s != "application/json" {
		return nil, fmt.Errorf("BatchApi: Expected application/json content; got %s", s)
	}

	// create new requestData to return in err
	d := &requestData{}
	d.method = req.Method
	d.uri = req.URL.RequestURI()
	d.header = make(http.Header)
	for k, s := range req.Header {
		if skip, _ := skipHeaders[k]; !skip {
			d.header[k] = s
		}
	}
	// add body if available
	if req.Body != nil {
		defer req.Body.Close()
		d.body = make([]byte, req.ContentLength, req.ContentLength)
		if _, err := req.Body.Read(d.body); err != nil {
			return nil, err
		}
	}
	return nil, d
}

// Request stores data from a client api call and is used by the Batch Service to
// create individual parts in the batch call
type Request struct {
	data *requestData // data used to created part during batch call

	status requestStatus

	resultPtr interface{} // ptr to null struct for use in json decode

	tag interface{} // extra data passed to Response to allow for
	// processing after batch completion

	credentialer Credentialer // if not nil then this credential overrides the
	//batch.Service credential
}

// String returns a string representation of the Request's tag.  Used for debugging
func (r *Request) String() string {
	return fmt.Sprintf("%v", r.tag)
}

// Response returns results of Request Call
type Response struct {
	// Result is the value of the decoded json
	Result interface{}
	// Tag is copied from corresponding Request.tag and is used to pass data
	// for further processing of the result.
	Tag interface{}
	// Err contains error response
	Err error
	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse
}

// Service for submitting batch
type Service struct {
	// Is set to http.DefaultClient if nil.  An oauth2.Client may be used
	// to authorize the requests or each individual request may have its
	// own credential removing the need for an authorizing client.
	Client      *http.Client
	MaxRequests int
	// if not nil, DoCtx() calls this during the request and response
	// assigning the []byte variable to the request or response body.
	DebugFunc func(string, []byte)
	// mu protectes the requests slice
	mu           sync.Mutex
	requests     []*Request
	initBuffSize int
}

// AddRequest adds a Request to the service with the corresponding options.  See
// example:
//
// ret, err = svCal.Events.Insert(*cal, eventData).Do()
// if err = batchService.AddItem(err,
//			batch.SetResult(&ret),
//			batch.SetTag(ExtraData),
//			batch.SetToken); err != nil {
//	  // handle error
// }
func (s *Service) AddRequest(e error, opts ...RequestOption) (err error) {
	var r = &Request{}
	if e == nil {
		return errors.New("BatchApi: Request was Called not Batched.  Service transport was not set to batch.Client()")
	}
	err = e
	// http.Client wraps underlying error in a url.Error so unwrap url.Error
	switch ex := err.(type) {
	case *requestData:
		r.data = ex // e.(*Request)
	case *url.Error:
		r.data, _ = ex.Err.(*requestData)
	}
	// if not a form of requestData then return original error
	if r.data == nil {
		return
	}
	// Process options and return error
	for _, o := range opts {
		if err = o(r); err != nil {
			return
		}
	}
	// Add to service request queue
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
	}()

	s.requests = append(s.requests, r)
	r.status = requestStatusQueued
	return nil
}

// Count returns number of requests currently batched
func (s *Service) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

// RequestList returns the current list of requests
func (s *Service) RequestList() []*Request {
	return s.requests
}

// Do sends up to maxRequests(default 1000) in a single request.  Remaining requests
// still stored for later calls
func (s *Service) Do() ([]Response, error) {
	return s.DoCtx(context.Background())
}

// DoCtx adds a context to the call
func (s *Service) DoCtx(ctx context.Context) ([]Response, error) {
	if len(s.requests) == 0 {
		return nil, errors.New("BatchApi: No requests queued")
	}
	s.mu.Lock()
	isLocked := true
	defer func() {
		if isLocked {
			s.mu.Unlock()
		}
	}()
	reqLen := len(s.requests)

	// Copy requests to local variable and reset s.requests
	// TODO: more eloquent error handling and setting of options
	maxRequests := s.MaxRequests
	requests := s.requests
	if maxRequests < 1 || maxRequests > 1000 {
		maxRequests = 1000
	}
	client := s.Client
	if client == nil {
		// using DefautClient is ok if individual Requests have a tokenSource
		client = http.DefaultClient
	}

	if reqLen > maxRequests {
		s.requests = s.requests[maxRequests:]
		requests = requests[:maxRequests-1]
	} else {
		s.requests = make([]*Request, 0, maxRequests)
	}
	s.mu.Unlock()
	isLocked = false

	outputBuf := bytes.NewBuffer(make([]byte, 0, s.initBuffSize))
	batchWriter := multipart.NewWriter(outputBuf)
	boundary := "multipart/mixed; boundary=\"" + batchWriter.Boundary() + "\""
	for cnt, r := range requests {
		pdata := r.data
		m := textproto.MIMEHeader{
			"Content-Type": []string{"application/http"},
			"Content-Id":   []string{fmt.Sprintf("batch%04d", cnt)},
		}
		// create part writer and write out headers
		pw, err := batchWriter.CreatePart(m)
		if err != nil {
			return nil, batchError(r, err)
		}

		pw.Write([]byte(pdata.method + " " + pdata.uri + " HTTP/1.1\r\n"))
		// add content-length header
		if len(pdata.body) > 0 {
			pdata.header.Set("Content-Length", strconv.Itoa(len(pdata.body)))
		}
		// obtain token if available
		if r.credentialer != nil {
			auth, err := r.credentialer.Authorization()
			if err != nil {
				return nil, batchError(r, err)
			}
			pdata.header.Set("Authorization", auth)
		}
		// write headers
		if err = pdata.header.WriteSubset(pw, skipHeaders); err != nil {
			return nil, batchError(r, err)
		}
		// blank line then full body
		pw.Write([]byte("\r\n"))
		if _, err = pw.Write(pdata.body); err != nil {
			return nil, batchError(r, err)
		}
	}
	batchWriter.Close()

	if s.DebugFunc != nil {
		s.DebugFunc("Request", outputBuf.Bytes())
	}

	// Create req to send batches
	req, err := http.NewRequest("POST", baseURL, outputBuf)
	if err != nil {
		return nil, err
	}
	req.ContentLength = int64(outputBuf.Len())
	req.Header.Set("Content-Type", boundary)
	req.Header.Set("User-Agent", "google-api-go-batch 0.1")

	resp, err := ctxhttp.Do(ctx, client, req)
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(resp)
	if err = googleapi.CheckResponse(resp); err != nil {
		return nil, err
	}
	cType, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(cType, "multipart/") {
		return nil, fmt.Errorf("BatchApi: Invalid Content Type returned %s", cType)
	}
	var body io.Reader = resp.Body
	if s.DebugFunc != nil {
		debugBuffer, err := ioutil.ReadAll(body)
		if err != nil {
			return nil, err
		}
		s.DebugFunc("Response", debugBuffer)
		body = bytes.NewReader(debugBuffer)
	}
	return ProcessBody(ctx, resp.Body, params["boundary"], requests)
}

// ProcessBody loops through requests and processes each part of multipart response
func ProcessBody(ctx context.Context, rc io.Reader, boundary string, requests []*Request) (results []Response, err error) {
	results = make([]Response, 0, len(requests))
	mr := multipart.NewReader(rc, boundary)

	// Http response from batch Do is supposed to be in the same order as results.
	for idx, req := range requests {
		// Check ct
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		pr, err := mr.NextPart()
		if err != nil {
			return nil, fmt.Errorf("Unable to parse multipart response(%d): %v", idx, err)
		}
		results = append(results, parseMimePart(pr, req))
	}
	return
}

// parseMimePart returns a Response from a mimepart.  See documentation at
// https://cloud.google.com/storage/docs/json_api/v1/how-tos/batch#response-to-a-batch-request
func parseMimePart(pr *multipart.Part, rq *Request) Response {
	defer pr.Close()
	var err error
	var res *http.Response

	if pr.Header.Get("Content-Type") != "application/http" {
		err = fmt.Errorf("Batch Api: Invalid Content Type: %s", pr.Header.Get("Content-Type"))
	} else if res, err = http.ReadResponse(bufio.NewReader(pr), nil); err == nil {
		defer res.Body.Close()
		err = googleapi.CheckResponse(res)
	}
	if err != nil {
		return ErrorResponse(rq, err)
	}
	result := NewResponse(res.Body, rq)
	result.ServerResponse =
		googleapi.ServerResponse{
			HTTPStatusCode: res.StatusCode,
			Header:         res.Header,
		}
	return result
}

// ErrorResponse creates a response with the passed error
func ErrorResponse(rq *Request, err error) Response {
	var ix interface{}
	if rq.resultPtr != nil {
		// Set result to a nil pointer to prevent casting errors on return
		ix = reflect.ValueOf(rq.resultPtr).Elem().Interface()
	}
	return Response{Result: ix, Err: err, Tag: rq.tag}
}

// NewResponse creates a response from a stream
func NewResponse(r io.Reader, rq *Request) Response {
	var ix interface{}
	var err error
	if rq.resultPtr != nil {
		// Set result to a nil pointer to prevent casting errors on return
		ix = reflect.ValueOf(rq.resultPtr).Elem().Interface()
		if r != nil {
			err = json.NewDecoder(r).Decode(rq.resultPtr)
		} else {
			err = errors.New("No JSON data returned")
		}
		//JSON Decode successful
		ix = reflect.ValueOf(rq.resultPtr).Elem().Interface()
	}
	return Response{
		Result: ix,
		Err:    err,
		Tag:    rq.tag,
	}
}

// RequestOption configurs
type RequestOption func(*Request) error

// SetResult provides a struct pointer so that batch.Request may unmarshall
// corresponding JSON response. Teh easiest way is to
// This should be set to the address of the value returned in the client api
// Do() call.  Not needed if client api call returns only an error
//
// ret, err := svCal.Events.Insert(*cal, eventData).Do()
// if err = batchService.AddItem(err, batch.SetResult(&ret)); err != nil {
//	  // handle error
// }
func SetResult(resultPtr interface{}) RequestOption {
	return func(r *Request) error {
		if resultPtr != nil {
			v := reflect.ValueOf(resultPtr)
			//Insist that ret is a ptr to ptr for json decoding
			if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Ptr {
				return errors.New("BatchApi: Invalid Result Pointer Value.  Must be a  **struct")
			}
		}
		r.resultPtr = resultPtr
		return nil
	}
}

// SetTag adds identifying data to the batch.Request which is then transfered
// to the corresponding response.  It may be used to allow for further processing
// of the response
func SetTag(tag interface{}) RequestOption {
	return func(r *Request) error {
		r.tag = tag
		return nil
	}
}

// SetCredentials overrides the batch.Service authorization.  This allows for multiple
// credential to be used in a single batch call.  Not needed if the request
// uses authorization from batch.Service
func SetCredentials(cred Credentialer) RequestOption {
	return func(r *Request) error {
		r.credentialer = cred
		return nil
	}
}
