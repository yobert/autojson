package autojson

import (
	"bytes"
	"errors"
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
func (s Service) E1() error {
	return errors.New("hi1")
}
func (s Service) E2() (error, int) {
	return errors.New("hi2"), 666
}
func (s Service) Empty() {
}
func (s Service) CodeOnly() int {
	return 666
}
func (s Service) CodeWithResp() (int, int) {
	return 666, 1234
}

func TestNewHandler(t *testing.T) {

	type handlertest struct {
		ServiceMethod string
		RequestBody   string
		Expect        string
		ExpectCode    int
	}
	tests := []handlertest{
		handlertest{"M1", "", "\"Hi\"\n", 200},
		handlertest{"M2", "\"sup\"", "{\"hello\":\"sup\"}\n", 200},
		handlertest{"M3", "", "true\n", 200},
		handlertest{"M4", "true", "true\n", 200},
		handlertest{"M4", "false", "false\n", 200},
		handlertest{"E1", "", "hi1\n", 500},
		handlertest{"E2", "", "hi2\n", 666},
		handlertest{"Empty", "", "", 204},
		handlertest{"CodeOnly", "", "", 666},
		handlertest{"CodeWithResp", "", "1234\n", 666},
	}

	var (
		mux     http.ServeMux
		service Service
	)

	for i, tt := range tests {
		mux.HandleFunc(fmt.Sprintf("/test/%d/", i), NewHandler(service, tt.ServiceMethod))
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
		endpoint := fmt.Sprintf("/test/%d/", tti)

		t.Run(fmt.Sprintf("NewEndpoint test %d: %s()", tti, tt.ServiceMethod), func(t *testing.T) {

			var (
				resp *http.Response
				err  error
			)

			if tt.RequestBody != "" {
				resp, err = client.Post("http://"+server.Addr+endpoint, "application/json", bytes.NewBufferString(tt.RequestBody))
			} else {
				resp, err = client.Get("http://" + server.Addr + endpoint)
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

			if resp.StatusCode != tt.ExpectCode {
				t.Errorf("Expected HTTP %d, got %d", tt.ExpectCode, resp.StatusCode)
			}

			if string(rv) != tt.Expect {
				t.Errorf("Expected %#v, got %#v", tt.Expect, string(rv))
			}
		})
	}
}
