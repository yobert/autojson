package autojson

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"reflect"
)

var (
	type_error    = reflect.TypeOf((*error)(nil)).Elem()
	type_context  = reflect.TypeOf((*context.Context)(nil)).Elem()
	type_int      = reflect.TypeOf(666)
	type_http_res = reflect.TypeOf((*http.ResponseWriter)(nil)).Elem()
	type_http_req = reflect.TypeOf(http.Request{})
)

func NewHandler(service_i interface{}, method_name string) http.HandlerFunc {
	service_v := reflect.ValueOf(service_i)
	service_t := service_v.Type()

	method, ok := service_t.MethodByName(method_name)
	if !ok {
		panic(fmt.Errorf("NewHandler(%s, %#v) type %s has no method %#v", service_t.String(), method_name, service_t.String(), method_name))
	}

	method_fv := method.Func
	method_ft := method.Type

	in_ctx := -1
	in_req := -1
	in_http_req := -1
	in_http_res := -1

	out_res := -1
	out_err := -1
	out_code := -1

	for i := 1; i < method_ft.NumIn(); i++ {
		in := method_ft.In(i)

		if in_ctx == -1 && in == type_context {
			in_ctx = i
			continue
		}
		if in_http_req == -1 && in == type_http_req {
			in_http_req = i
			continue
		}
		if in_http_res == -1 && in == type_http_res {
			in_http_res = i
			continue
		}
		// assume any leftover argment is a request
		if in_req == -1 {
			in_req = i
			continue
		}

		panic(fmt.Errorf("NewHandler(%s, %#v) too many arguments: Not sure how to populate argument %d (%s)", service_t.String(), method_name, i-1, in))
	}

	for i := 0; i < method_ft.NumOut(); i++ {
		out := method_ft.Out(i)

		if out_err == -1 && out == type_error {
			out_err = i
			continue
		}
		if out_code == -1 && out == type_int {
			out_code = i
			continue
		}
		if out_res == -1 {
			out_res = i
			continue
		}

		panic(fmt.Errorf("NewHandler(%s, %#v) too many return values: Not sure what to do with value %d (%s)", service_t.String(), method_name, i, out))
	}

	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		args := make([]reflect.Value, method_ft.NumIn())
		args[0] = service_v

		if in_ctx != -1 {
			args[in_ctx] = reflect.ValueOf(r.Context())
		}
		if in_http_req != -1 {
			args[in_http_req] = reflect.ValueOf(r)
		}
		if in_http_res != -1 {
			args[in_http_res] = reflect.ValueOf(w)
		}
		if in_req != -1 {
			in := method_ft.In(in_req)

			in_v := reflect.New(in)
			in_i := in_v.Interface()

			d := json.NewDecoder(r.Body)

			err := d.Decode(in_i)

			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			if in.Kind() == reflect.Ptr {
				args[in_req] = in_v
			} else {
				args[in_req] = in_v.Elem()
			}
		}

		res_vals := method_fv.Call(args)

		var (
			v_out_err  error
			v_out_code int
			v_out_res  interface{}
		)

		if out_code != -1 {
			v_out_code, _ = res_vals[out_code].Interface().(int)
		}
		if out_err != -1 {
			v_out_err, _ = res_vals[out_err].Interface().(error)
		}
		if out_res != -1 {
			v_out_res = res_vals[out_res].Interface()
		}

		// if you set http return code -1, that means
		// don't return a response. use this if you want
		// to completely skip JSON encoding, upgrade a websocket,
		// etc.
		if v_out_code == -1 {
			return
		}

		// if you don't specify a http code, default to 500 or 200
		if v_out_code == 0 {
			if v_out_err == nil {
				if out_res == -1 {
					v_out_code = 204
				} else {
					v_out_code = 200
				}
			} else {
				v_out_code = 500
			}
		}

		if v_out_err != nil {
			http.Error(w, v_out_err.Error(), v_out_code)
			return
		}

		// if you don't have a return value at all, return an empty response body.
		// (as opposed to "null", if you return nil).  in this case, don't return
		// a content type.
		if out_res == -1 {
			w.WriteHeader(v_out_code)
			return
		}

		// return a JSON encoded response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(v_out_code)

		if err := json.NewEncoder(w).Encode(v_out_res); err != nil {
			log.Printf("JSON encode error: %v: Cannot send response back to client\n", err)
			return
		}

		// Success!
		return
	}
}
