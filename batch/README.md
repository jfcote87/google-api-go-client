Google Client Api Batch Utility in Go

The batch package allows your code to queue client api calls from various client api go packages
and send the queued calls as a single batch call.  Individual responses may then be interrogated 
for errors and the responses processed.  For specifics of the batch protocol see
https://cloud.google.com/storage/docs/json_api/v1/how-tos/batch

The first part of the package is the batch.BatchClient which is a mock http.Client that intercepts
client api Do() calls and returns an error that may be parsed into a batch request. The batch.Service
struct collects the parses the error using the AddRequest(err,...) func.  Queued requests are posted
via the Do() method which returns a slice Response objects.  

Examples may be found in the example_test.go file.