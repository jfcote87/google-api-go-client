Google Client Api Batch Utility in Go

The batch package allows your code to queue client api calls from various client api go packages
and send the queued calls via a single batch call.  Individual responses may then be interrogated 
for errors and the responses processed.  For specifics of the batch protocol see
[Link](https://cloud.google.com/storage/docs/json_api/v1/how-tos/batch)

batch.BatchClient which is a mock http.Client that intercepts
client api Do() calls and returns an error that may be parsed 
into a batch request. A batch.Service parses the error using the 
AddRequest(err, ...RequestOption) func and queues the Request.
Queued requests are posted via the Do() method which returns a 
slice Response objects. 

Examples may be found in the example_test.go file.