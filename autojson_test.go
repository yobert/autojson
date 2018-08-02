package autojson

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
)

type Service struct {
}

type M2Resp struct {
	Hello string `json:"hello"`
}

func (s Service) M1() (string, error) {
	return "Hi", nil
}
func (s Service) M2(hi string) (M2Resp, error) {
	return M2Resp{
		Hello: hi,
	}, nil
}
func (s Service) M3() (bool, error) {
	return true, nil
}
func (s Service) M4(v bool) (bool, error) {
	return v, nil
}

func TestNewHandler(t *testing.T) {

	type handlertest struct {
		Endpoint      string
		ServiceMethod string
		RequestBody   string
		Expect        string
	}
	tests := []handlertest{
		handlertest{"/m1", "M1", "", "\"Hi\"\n"},
		handlertest{"/m2", "M2", "\"sup\"", "{\"hello\":\"sup\"}\n"},
		handlertest{"/m3", "M3", "", "true\n"},
		handlertest{"/m4a", "M4", "true", "true\n"},
		handlertest{"/m4b", "M4", "false", "false\n"},
	}

	var (
		mux     http.ServeMux
		service Service
	)

	for _, tt := range tests {
		mux.HandleFunc(tt.Endpoint, NewHandler(service, tt.ServiceMethod))
	}

	server := http.Server{
		Addr:    "localhost:6666",
		Handler: &mux,
	}

	go func() {
		err := server.ListenAndServe()
		if err != nil {
			fmt.Println(err)
			return
		}
	}()

	client := http.Client{}

	for tti, tt := range tests {
		t.Run(fmt.Sprintf("NewEndpoint test %d (%#v, %#v)", tti, tt.Endpoint, tt.ServiceMethod), func(t *testing.T) {

			var (
				resp *http.Response
				err  error
			)

			if tt.RequestBody != "" {
				resp, err = client.Post("http://"+server.Addr+tt.Endpoint, "application/json", bytes.NewBufferString(tt.RequestBody))
			} else {
				resp, err = client.Get("http://" + server.Addr + tt.Endpoint)
			}
			if err != nil {
				t.Error(err)
				return
			}

			rv, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Error(err)
				return
			}

			if string(rv) != tt.Expect {
				t.Errorf("Expected %#v, got %#v", tt.Expect, string(rv))
				return
			}
		})
	}
}
