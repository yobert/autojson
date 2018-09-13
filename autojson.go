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

type in_idx struct {
	ctx      int
	req      int
	http_req int
	http_res int
}

type out_idx struct {
	res  int
	err  int
	code int
}

func reflect_in(f reflect.Type) (in_idx, error) {
	r := in_idx{
		ctx:      -1,
		req:      -1,
		http_req: -1,
		http_res: -1,
	}
	for i := 1; i < f.NumIn(); i++ {
		in := f.In(i)

		if r.ctx == -1 && in == type_context {
			r.ctx = i
			continue
		}
		if r.http_req == -1 && in == type_http_req {
			r.http_req = i
			continue
		}
		if r.http_res == -1 && in == type_http_res {
			r.http_res = i
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
func reflect_out(f reflect.Type) (out_idx, error) {
	r := out_idx{
		res:  -1,
		err:  -1,
		code: -1,
	}
	for i := 0; i < f.NumOut(); i++ {
		out := f.Out(i)

		if r.err == -1 && out == type_error {
			r.err = i
			continue
		}
		if r.code == -1 && out == type_int {
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

func NewHandler(service_i interface{}, method_name string) http.HandlerFunc {
	service_v := reflect.ValueOf(service_i)
	service_t := service_v.Type()

	method, ok := service_t.MethodByName(method_name)
	if !ok {
		panic(fmt.Errorf("NewHandler(%s, %#v) type %s has no method %#v", service_t.String(), method_name, service_t.String(), method_name))
	}

	method_fv := method.Func
	method_ft := method.Type

	in, err := reflect_in(method_ft)
	if err != nil {
		panic(fmt.Errorf("NewHandler(%s, %#v) %v", service_t.String(), method_name, err))
	}
	out, err := reflect_out(method_ft)
	if err != nil {
		panic(fmt.Errorf("NewHandler(%s, %#v) %v", service_t.String(), method_name, err))
	}

	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		args := make([]reflect.Value, method_ft.NumIn())
		args[0] = service_v

		if in.ctx != -1 {
			args[in.ctx] = reflect.ValueOf(r.Context())
		}
		if in.http_req != -1 {
			args[in.http_req] = reflect.ValueOf(r)
		}
		if in.http_res != -1 {
			args[in.http_res] = reflect.ValueOf(w)
		}
		if in.req != -1 {
			in_f := method_ft.In(in.req)
			in_v := reflect.New(in_f)
			in_i := in_v.Interface()

			d := json.NewDecoder(r.Body)

			err := d.Decode(in_i)

			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			if in_f.Kind() == reflect.Ptr {
				args[in.req] = in_v
			} else {
				args[in.req] = in_v.Elem()
			}
		}

		res_vals := method_fv.Call(args)

		var (
			v_out_err  error
			v_out_code int
			v_out_res  interface{}
		)

		if out.code != -1 {
			v_out_code, _ = res_vals[out.code].Interface().(int)
		}
		if out.err != -1 {
			v_out_err, _ = res_vals[out.err].Interface().(error)
		}
		if out.res != -1 {
			v_out_res = res_vals[out.res].Interface()
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
				if out.res == -1 {
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
		if out.res == -1 {
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
