package autojson

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"strconv"
)

type argsIndex struct {
	ctx     int
	req     int
	httpReq int
	httpRes int
}

type returnsIndex struct {
	res  int
	err  int
	code int
}

// ErrorResponse will be encoded as JSON and returned if your handler has a
// return value of type error, with the stringified error populated.
type ErrorResponse struct {
	Error string `json:"error"`
}

func reflectArgs(f reflect.Type) (argsIndex, error) {
	r := argsIndex{
		ctx:     -1,
		req:     -1,
		httpReq: -1,
		httpRes: -1,
	}
	for i := 1; i < f.NumIn(); i++ {
		in := f.In(i)

		if r.ctx == -1 && in == reflect.TypeOf((*context.Context)(nil)).Elem() {
			r.ctx = i
			continue
		}
		if r.httpReq == -1 && in == reflect.TypeOf(&http.Request{}) {
			r.httpReq = i
			continue
		}
		if r.httpRes == -1 && in == reflect.TypeOf((*http.ResponseWriter)(nil)).Elem() {
			r.httpRes = i
			continue
		}
		// assume any leftover argment is a request
		if r.req == -1 {
			r.req = i
			continue
		}
		return r, fmt.Errorf("Too many arguments: Not sure how to populate argument %d (%s)", i-1, in)
	}
	return r, nil
}
func reflectReturns(f reflect.Type) (returnsIndex, error) {
	r := returnsIndex{
		res:  -1,
		err:  -1,
		code: -1,
	}
	for i := 0; i < f.NumOut(); i++ {
		out := f.Out(i)

		if r.err == -1 && out == reflect.TypeOf((*error)(nil)).Elem() {
			r.err = i
			continue
		}
		if r.code == -1 && out == reflect.TypeOf(666) {
			r.code = i
			continue
		}
		if r.res == -1 {
			r.res = i
			continue
		}
		return r, fmt.Errorf("Too many return values: Not sure what to do with value %d (%s)", i, out)
	}
	return r, nil
}

// NewHandler uses reflection to generate an http.HandlerFunc from a service and method name
func NewHandler(service interface{}, methodName string) http.HandlerFunc {
	serviceVal := reflect.ValueOf(service)
	serviceType := serviceVal.Type()

	method, ok := serviceType.MethodByName(methodName)
	if !ok {
		panic(fmt.Errorf("NewHandler(%s, %#v) type %s has no method %#v", serviceType.String(), methodName, serviceType.String(), methodName))
	}

	in, err := reflectArgs(method.Type)
	if err != nil {
		panic(fmt.Errorf("NewHandler(%s, %#v) %v", serviceType.String(), methodName, err))
	}
	out, err := reflectReturns(method.Type)
	if err != nil {
		panic(fmt.Errorf("NewHandler(%s, %#v) %v", serviceType.String(), methodName, err))
	}

	return buildHandler(in, out, serviceVal, method)
}

func buildHandler(in argsIndex, out returnsIndex, service reflect.Value, method reflect.Method) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		args := make([]reflect.Value, method.Type.NumIn())
		args[0] = service

		if in.ctx != -1 {
			args[in.ctx] = reflect.ValueOf(r.Context())
		}
		if in.httpReq != -1 {
			args[in.httpReq] = reflect.ValueOf(r)
		}
		if in.httpRes != -1 {
			args[in.httpRes] = reflect.ValueOf(w)
		}
		if in.req != -1 {
			inType := method.Type.In(in.req)
			inValue := reflect.New(inType)
			inInterface := inValue.Interface()

			d := json.NewDecoder(r.Body)

			err := d.Decode(inInterface)

			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			args[in.req] = inValue.Elem()
		}

		res := method.Func.Call(args)

		var (
			outErr  error
			outCode int
			outRes  interface{}
		)

		if out.code != -1 {
			outCode, _ = res[out.code].Interface().(int)
		}
		if out.err != -1 {
			outErr, _ = res[out.err].Interface().(error)
		}
		if out.res != -1 {
			outRes = res[out.res].Interface()
		}

		// if you set http return code -1, that means
		// don't return a response. use this if you want
		// to completely skip JSON encoding, upgrade a websocket,
		// etc.
		if outCode == -1 {
			return
		}

		// if you don't specify a http code, default to 500 or 200
		if outCode == 0 {
			if outErr == nil {
				outCode = 200
			} else {
				outCode = 500
			}
		}

		if outErr != nil {
			outRes = &ErrorResponse{Error: outErr.Error()}
		}

		// If your handler returns something that cannot be marshalled
		// to valid JSON, we're going to return an error and override
		// any requested status code to 500.
		//
		// Pros of encoding JSON to a buffer first:
		// - We can send a correct Content-Length so net/http doesn't have
		//   to do any games with our output
		// - We can capture this type of error and give a nice reply instead
		//   of just closing the response
		//
		// Cons:
		// - A huge data structure must be buffered in memory first
		// - An object with a special encoding method could have streamed
		//   bytes to the client.  (Super cool, but not common at all.)
		buf, err := json.Marshal(outRes)
		if err != nil {
			// We still want this error to be JSON
			outCode = 500
			outRes = &ErrorResponse{Error: err.Error()}
			buf, err = json.Marshal(outRes)

			if err != nil {
				// Well, shit
				log.Printf("Error encoding error to JSON: %v\n", err)
				http.Error(w, err.Error(), 500)
				return
			}

			// okay we can send the encoded error below
		}

		w.Header().Set("Content-Length", strconv.Itoa(len(buf)))
		w.Header().Set("Content-Type", "application/json")

		w.WriteHeader(outCode)

		if _, err := w.Write(buf); err != nil {
			log.Printf("Write error: %v\n", err)
			return
		}

		// Success!
		return
	}
}
