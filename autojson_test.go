package autojson

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
)

const testHeaderName = "X-Header-Test"

type Things struct {
	One string `json:"one"`
	Two string `json:"two"`
}
type Unencodable chan int

type Service struct {
}

type M2Resp struct {
	Hello string `json:"hello"`
}

func (Service) M1() (string, error) {
	return "Hi", nil
}
func (Service) M2(hi string) (M2Resp, error) {
	return M2Resp{
		Hello: hi,
	}, nil
}
func (Service) M3() (*bool, error) {
	b := true
	return &b, nil
}
func (Service) M4(v bool) (bool, error) {
	return v, nil
}
func (Service) M5(in Things) Things {
	return Things{One: in.Two, Two: in.One}
}
func (Service) M6(in *Things) *Things {
	return &Things{One: in.Two, Two: in.One}
}
func (Service) E1() error {
	return errors.New("hi1")
}
func (Service) E2() (int, error) {
	return 666, errors.New("hi2")
}
func (Service) Numbers(n int) (int, int) {
	return 200, n
}
func (Service) Empty() {
}
func (Service) CodeOnly() int {
	return 666
}
func (Service) CodeWithResp() (int, int) {
	return 666, 1234
}
func (Service) HeaderTest(w http.ResponseWriter) string {
	w.Header().Set(testHeaderName, "coolio")
	return "wasaaap"
}
func (Service) ContextTest(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	return true
}
func (Service) CustomResponse(w http.ResponseWriter, r *http.Request) int {
	// just to make sure it's populated correctly
	_ = r.Header

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(201)
	w.Write([]byte("A plain text response"))
	return -1
}
func (Service) Unencodable() Unencodable {
	var a Unencodable
	return a
}

func (Service) BadRequest(in string) {
}

// This should panic because you can't deserialize one body request into multiple arguments
func (Service) TooManyArguments(a, b string) {
}

// This should panic because you can't serialize one response into multiple return values
func (Service) TooManyValues() (string, string) {
	return "", ""
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
		{"M1", "", "\"Hi\"", 200, ""},
		{"M2", "\"sup\"", "{\"hello\":\"sup\"}", 200, ""},
		{"M3", "", "true", 200, ""},
		{"M4", "true", "true", 200, ""},
		{"M4", "false", "false", 200, ""},
		{"M5", "{\"one\":\"a\", \"two\":\"b\"}", "{\"one\":\"b\",\"two\":\"a\"}", 200, ""},
		{"M6", "{\"one\":\"a\", \"two\":\"b\"}", "{\"one\":\"b\",\"two\":\"a\"}", 200, ""},
		{"E1", "", "{\"error\":\"hi1\"}", 500, ""},
		{"E2", "", "{\"error\":\"hi2\"}", 666, ""},
		{"Numbers", "1234", "1234", 200, ""},
		{"Empty", "", "null", 200, ""},
		{"CodeOnly", "", "null", 666, ""},
		{"CodeWithResp", "", "1234", 666, ""},
		{"HeaderTest", "", "\"wasaaap\"", 200, "coolio"},
		{"ContextTest", "", "true", 200, ""},
		{"CustomResponse", "", "A plain text response", 201, ""},
		{"Unencodable", "", "{\"error\":\"json: unsupported type: autojson.Unencodable\"}", 500, ""},
		{"BadRequest", "yo", "invalid character 'y' looking for beginning of value\n", 400, ""},
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
	defer server.Shutdown(context.Background())

	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			fmt.Println(err)
			return
		}
	}()

	client := http.Client{}

	for tti, tt := range tests {
		endpoint := fmt.Sprintf("/test/%d/", tti)

		t.Run(fmt.Sprintf("NewHandler() test %d: %s()", tti, tt.ServiceMethod), func(t *testing.T) {

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

func TestNewHandlerPanics(t *testing.T) {

	var service Service

	type handlertest struct {
		ServiceMethod string
		Panic         string
	}

	tests := []handlertest{
		{"TooManyArguments", "Too many arguments"},
		{"TooManyValues", "Too many return values"},
		{"Lard", "has no method \"Lard\""},
	}

	for tti, tt := range tests {
		t.Run(fmt.Sprintf("NewHandler() panic test %d: %s()", tti, tt.ServiceMethod), func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Errorf("Expected panic %#v not found", tt.Panic)
					return
				}
				pstr := ""
				switch v := r.(type) {
				case error:
					pstr = v.Error()
				case string:
					pstr = v
				default:
					t.Errorf("Expected panic %#v, got unhandled panic type", tt.Panic)
				}
				if strings.Index(pstr, tt.Panic) > -1 {
					// good!
					return
				}
				t.Errorf("Expected panic %#v, got %#v", tt.Panic, pstr)
			}()

			_ = NewHandler(service, tt.ServiceMethod)
		})
	}
}
