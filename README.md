[![Go Report Card](https://goreportcard.com/badge/github.com/yobert/autojson)](https://goreportcard.com/report/github.com/yobert/autojson)
[![Godoc](https://godoc.org/github.com/yobert/autojson?status.svg)](http://godoc.org/github.com/yobert/autojson)

This library will generate an HTTP HandlerFunc that takes JSON as input, and returns JSON as output.
Pass it an object, and a method name of that object as a string. You can have various combinations of
argments and return values to control behavior, depending on the value types. All are optional.

Method Argument Types
---------------------
An argument of type context.Context, http.ResponseWriter, http.Request will be copied from the source request.

Any remaining argment will be populated with data by encoding/json from the request body.

Method Return Types
-------------------
If you return a value of type error, it will be used to generate an HTTP 500 with a message if non-nil.

If you return an int, it will be used as the HTTP status code, overriding any code (including the 500 from an error).

If you return any other value, it will be marshalled by encoding/json and returned to the client.
