package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBodyLimitUsesUploadLimitForMatchingPath(t *testing.T) {
	var readErr error
	h := BodyLimit(10, 1<<20, "/dump/import")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
	}))

	body := strings.Repeat("a", 1000)
	req := httptest.NewRequest(http.MethodPost, "/api/connections/x/databases/y/dump/import", strings.NewReader(body))
	h.ServeHTTP(httptest.NewRecorder(), req)
	if readErr != nil {
		t.Fatalf("upload path should accept %d bytes: %v", len(body), readErr)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/connections/x/query", strings.NewReader(body))
	h.ServeHTTP(httptest.NewRecorder(), req)
	var tooLarge *http.MaxBytesError
	if !errors.As(readErr, &tooLarge) {
		t.Fatalf("non-upload path should hit the default limit, got %v", readErr)
	}
}
