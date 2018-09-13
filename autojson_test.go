package autojson

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
)

const testHeaderName = "X-Header-Test"

type Service struct {
}

type M2Resp struct {
	Hello string `json:"hello"`
}

func (_ Service) M1() (string, error) {
	return "Hi", nil
}
func (_ Service) M2(hi string) (M2Resp, error) {
	return M2Resp{
		Hello: hi,
	}, nil
}
func (_ Service) M3() (bool, error) {
	return true, nil
}
func (_ Service) M4(v bool) (bool, error) {
	return v, nil
}
func (_ Service) E1() error {
	return errors.New("hi1")
}
func (_ Service) E2() (int, error) {
	return 666, errors.New("hi2")
}
func (_ Service) Numbers(n int) (int, int) {
	return 200, n
}
func (_ Service) Empty() {
}
func (_ Service) CodeOnly() int {
	return 666
}
func (_ Service) CodeWithResp() (int, int) {
	return 666, 1234
}
func (_ Service) HeaderTest(w http.ResponseWriter) string {
	w.Header().Set(testHeaderName, "coolio")
	return "wasaaap"
}
func (_ Service) CustomResponse(w http.ResponseWriter, r *http.Request) int {
	// just to make sure it's populated correctly
	_ = r.Header

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(201)
	w.Write([]byte("A plain text response"))
	return -1
}

func TestNewHandler(t *testing.T) {

	type handlertest struct {
		ServiceMethod string
		RequestBody   string
		Expect        string
		ExpectCode    int
		HeaderTest    string
	}
	tests := []handlertest{
		{"M1", "", "\"Hi\"\n", 200, ""},
		{"M2", "\"sup\"", "{\"hello\":\"sup\"}\n", 200, ""},
		{"M3", "", "true\n", 200, ""},
		{"M4", "true", "true\n", 200, ""},
		{"M4", "false", "false\n", 200, ""},
		{"E1", "", "hi1\n", 500, ""},
		{"E2", "", "hi2\n", 666, ""},
		{"Numbers", "1234", "1234\n", 200, ""},
		{"Empty", "", "", 204, ""},
		{"CodeOnly", "", "", 666, ""},
		{"CodeWithResp", "", "1234\n", 666, ""},
		{"HeaderTest", "", "\"wasaaap\"\n", 200, "coolio"},
		{"CustomResponse", "", "A plain text response", 201, ""},
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
			rvs := string(rv)

			if resp.StatusCode != tt.ExpectCode {
				t.Errorf("Expected HTTP %d, got %d", tt.ExpectCode, resp.StatusCode)
			}

			if rvs != tt.Expect {
				t.Errorf("Expected %#v, got %#v", tt.Expect, rvs)
			}

			h := resp.Header.Get(testHeaderName)
			if h != tt.HeaderTest {
				t.Errorf("Expected header %#v to be %#v, got %#v", testHeaderName, tt.HeaderTest, h)
			}
		})
	}
}
