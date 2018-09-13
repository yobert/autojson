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
		if r.httpReq == -1 && in == reflect.TypeOf((*http.ResponseWriter)(nil)).Elem() {
			r.httpReq = i
			continue
		}
		if r.httpRes == -1 && in == reflect.TypeOf(http.Request{}) {
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

	methodFunc := method.Func
	methodType := method.Type

	in, err := reflectArgs(methodType)
	if err != nil {
		panic(fmt.Errorf("NewHandler(%s, %#v) %v", serviceType.String(), methodName, err))
	}
	out, err := reflectReturns(methodType)
	if err != nil {
		panic(fmt.Errorf("NewHandler(%s, %#v) %v", serviceType.String(), methodName, err))
	}

	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		args := make([]reflect.Value, methodType.NumIn())
		args[0] = serviceVal

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
			inType := methodType.In(in.req)
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

		res := methodFunc.Call(args)

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
				if out.res == -1 {
					outCode = 204
				} else {
					outCode = 200
				}
			} else {
				outCode = 500
			}
		}

		if outErr != nil {
			http.Error(w, outErr.Error(), outCode)
			return
		}

		// if you don't have a return value at all, return an empty response body.
		// (as opposed to "null", if you return nil).  in this case, don't return
		// a content type.
		if out.res == -1 {
			w.WriteHeader(outCode)
			return
		}

		// return a JSON encoded response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(outCode)

		if err := json.NewEncoder(w).Encode(outRes); err != nil {
			log.Printf("JSON encode error: %v: Cannot send response back to client\n", err)
			return
		}

		// Success!
		return
	}
}
