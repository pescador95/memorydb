package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewHandlerRejectsNilReadinessCheck(t *testing.T) {
	if _, err := NewHandler(nil); err != ErrNilReadinessCheck {
		t.Fatalf("NewHandler() error = %v, want %v", err, ErrNilReadinessCheck)
	}
}

func TestHandlerLivez(t *testing.T) {
	handler, err := NewHandler(func() bool {
		t.Fatal("liveness must not invoke the readiness check")
		return false
	})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/livez", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	if got := recorder.Body.String(); got != "ok\n" {
		t.Fatalf("body = %q, want %q", got, "ok\n")
	}

	if got := recorder.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want %q", got, "text/plain; charset=utf-8")
	}
}

func TestHandlerReadyz(t *testing.T) {
	tests := []struct {
		name           string
		ready          bool
		expectedStatus int
	}{
		{
			name:           "ready",
			ready:          true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "not ready",
			ready:          false,
			expectedStatus: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readinessChecks := 0

			handler, err := NewHandler(func() bool {
				readinessChecks++
				return tt.ready
			})
			if err != nil {
				t.Fatalf("NewHandler() error = %v", err)
			}

			request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, request)

			if recorder.Code != tt.expectedStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.expectedStatus)
			}

			if readinessChecks != 1 {
				t.Fatalf("readiness checks = %d, want 1", readinessChecks)
			}
		})
	}
}

func TestHandlerRejectsUnsupportedMethods(t *testing.T) {
	handler, err := NewHandler(func() bool {
		return true
	})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	tests := []struct {
		name   string
		method string
	}{
		{
			name:   "POST",
			method: http.MethodPost,
		},
		{
			name:   "PUT",
			method: http.MethodPut,
		},
		{
			name:   "DELETE",
			method: http.MethodDelete,
		},
		{
			name:   "HEAD",
			method: http.MethodHead,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(tt.method, "/livez", nil)
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusMethodNotAllowed {
				t.Fatalf(
					"status = %d, want %d",
					recorder.Code,
					http.StatusMethodNotAllowed,
				)
			}

			if got := recorder.Header().Get("Allow"); got != http.MethodGet {
				t.Fatalf("Allow = %q, want %q", got, http.MethodGet)
			}
		})
	}
}

func TestHandlerReturnsNotFoundForUnknownPath(t *testing.T) {
	handler, err := NewHandler(func() bool {
		return true
	})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}
