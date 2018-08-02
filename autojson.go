package autojson

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"reflect"
)

var an_error = reflect.TypeOf((*error)(nil)).Elem()
var a_context = reflect.TypeOf((*context.Context)(nil)).Elem()

func NewHandler(service_i interface{}, method_name string) http.HandlerFunc {
	service_v := reflect.ValueOf(service_i)
	service_t := service_v.Type()

	method, ok := service_t.MethodByName(method_name)
	if !ok {
		panic(fmt.Errorf("NewHandler(%s, %#v) type %s has no method %#v", service_t.String(), method_name, service_t.String(), method_name))
	}

	method_fv := method.Func
	method_ft := method.Type

	n := method_ft.NumIn()

	matched := false

	// sanity type check here, to try to fail early
	for i := 1; i < n; i++ {
		in := method_ft.In(i)

		// context.Context argument
		if i == 1 && in.Kind() == reflect.Interface && in.Implements(a_context) {
			continue
		}

		if matched {
			panic(fmt.Errorf("NewHandler(%s, %#v) too many arguments: Only a context argument and up to one argument for JSON decoding is allowed", service_t.String(), method_name))
		}

		matched = true
	}
	if method_ft.NumOut() != 2 {
		panic(fmt.Errorf("NewHandler(%s, %#v) method return type error: Must be (<Value>, error)", service_t.String(), method_name))
	}
	if !method_ft.Out(1).Implements(an_error) {
		panic(fmt.Errorf("NewHandler(%s, %#v) method second return argument does not satisfy error interface: %#v", service_t.String(), method_name, method_ft.Out(1).String()))
	}

	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		args := make([]reflect.Value, n)
		args[0] = service_v

		for i := 1; i < n; i++ {
			in := method_ft.In(i)

			// If the first argument is an interface that satisfies context.Context, it will be populated
			// with the context of the HTTP request.
			if i == 1 && in.Kind() == reflect.Interface && in.Implements(a_context) {
				args[i] = reflect.ValueOf(r.Context())
				continue
			}

			// Any other argument, we will attempt to populate with json.Unmarshal of the request body
			in_v := reflect.New(in)
			in_i := in_v.Interface()

			d := json.NewDecoder(r.Body)

			err := d.Decode(in_i)

			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			if in.Kind() == reflect.Ptr {
				args[i] = in_v
			} else {
				args[i] = in_v.Elem()
			}

			// there should be no other arguments.
			break
		}

		res_vals := method_fv.Call(args)

		if len(res_vals) != 2 {
			err := fmt.Errorf("NewHandler(%s, %#v) method return type error: Must be (<Value>, error)", service_t.String(), method_name)
			log.Println(err)
			http.Error(w, err.Error(), 500)
			return
		}

		res_v := res_vals[0]
		err_v := res_vals[1]

		if !err_v.Type().Implements(an_error) {
			err := fmt.Errorf("NewHandler(%s, %#v) method second return argument does not satisfy error interface: %#v", service_t.String(), method_name, err_v.Type().String())
			log.Println(err)
			http.Error(w, err.Error(), 500)
			return
		}

		if !err_v.IsNil() {
			http.Error(w, err_v.Interface().(error).Error(), 500)
			return
		}

		res_i := res_v.Interface()

		w.Header().Set("Content-Type", "application/json")

		e := json.NewEncoder(w)

		err := e.Encode(res_i)
		if err != nil {
			log.Printf("JSON encode error: %v: Cannot send response back to client\n", err)
			return
		}

		// Success!
		return
	}
}
