package autojson

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"reflect"
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

// ErrorResponse will be returned if your handler has a return value of type error,
// with the stringified error populated.
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

			if inType.Kind() == reflect.Ptr {
				args[in.req] = inValue
			} else {
				args[in.req] = inValue.Elem()
			}
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
		//
		// Otherwise, we will _always_ return valid JSON, with the
		// exception of if JSON encoding fails, in which case the
		// response will simply be truncated.
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

		w.Header().Set("Content-Type", "application/json")

		if outErr != nil {
			outRes = &ErrorResponse{Error: outErr.Error()}
		}

		// return a JSON encoded response
		w.WriteHeader(outCode)

		if err := json.NewEncoder(w).Encode(outRes); err != nil {
			log.Printf("JSON encode error: %v: Cannot send response back to client\n", err)
			return
		}

		// Success!
		return
	}
}
