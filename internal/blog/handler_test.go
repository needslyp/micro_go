package blog

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetByIDUsesPath(t *testing.T) {
	handler := &Handler{service: &Service{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /articles/{id}", handler.GetByID)
	request := httptest.NewRequest(http.MethodGet, "/articles/-1?id=1", nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func TestGetListRejectsMalformedPagination(t *testing.T) {
	for _, query := range []string{
		"limit=abc", "offset=", "limit=1&limit=2", "unknown=1", "limit=%zz",
	} {
		t.Run(query, func(t *testing.T) {
			// A malformed query must return before accessing the service.
			handler := &Handler{}
			request := httptest.NewRequest(http.MethodGet, "/articles?"+query, nil)
			response := httptest.NewRecorder()

			handler.GetList(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", response.Code)
			}
			var body map[string]string
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["error"] != "invalid pagination" {
				t.Fatalf("unexpected response: %v", body)
			}
		})
	}
}

func TestReadArticle(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want int
	}{
		{name: "valid", body: `{"title":"Title","body":"Text"}`, want: 200},
		{name: "author injection", body: `{"title":"Title","body":"Text","author_id":99}`, want: 400},
		{name: "missing", body: `{"title":"Title"}`, want: 400},
		{name: "null", body: `{"title":null,"body":"Text"}`, want: 400},
		{name: "wrong type", body: `{"title":1,"body":"Text"}`, want: 400},
		{name: "extra value", body: `{"title":"Title","body":"Text"} {}`, want: 400},
		{name: "oversized", body: `{"title":"Title","body":"` + strings.Repeat("x", 1<<20) + `"}`, want: 413},
		{name: "oversized trailing whitespace", body: `{"title":"Title","body":"Text"}` + strings.Repeat(" ", 1<<20), want: 413},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/articles", strings.NewReader(tt.body))
			_, ok := readArticle(response, request)
			if ok != (tt.want == 200) || response.Code != tt.want {
				t.Fatalf("accepted=%v status=%d, want %d", ok, response.Code, tt.want)
			}
		})
	}
}

func TestRouterJSONErrors(t *testing.T) {
	router := newRouter(&Handler{}, nil)
	for _, tt := range []struct {
		method  string
		path    string
		status  int
		allow   string
		message string
	}{
		{method: "GET", path: "/missing", status: 404, message: "not found"},
		{method: "POST", path: "/missing", status: 404, message: "not found"},
		{method: "GET", path: "/articles/1/extra", status: 404, message: "not found"},
		{method: "PATCH", path: "/articles", status: 405, allow: "GET, HEAD, POST", message: "method not allowed"},
		{method: "POST", path: "/articles/1", status: 405, allow: "DELETE, GET, HEAD, PUT", message: "method not allowed"},
		{method: "OPTIONS", path: "/articles/1", status: 405, allow: "DELETE, GET, HEAD, PUT", message: "method not allowed"},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(tt.method, tt.path, nil))
			if response.Code != tt.status {
				t.Fatalf("status=%d, want %d", response.Code, tt.status)
			}
			if response.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("unexpected Content-Type: %s", response.Header().Get("Content-Type"))
			}
			if response.Header().Get("Allow") != tt.allow {
				t.Fatalf("Allow=%q, want %q", response.Header().Get("Allow"), tt.allow)
			}
			var body map[string]string
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["error"] != tt.message {
				t.Fatalf("unexpected error: %v", body)
			}
		})
	}
}
