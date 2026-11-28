package health

import (
	"errors"
	"net/http"
)

var ErrNilReadinessCheck = errors.New("health readiness check is nil")

type Handler struct {
	ready func() bool
}

func NewHandler(ready func() bool) (*Handler, error) {
	if ready == nil {
		return nil, ErrNilReadinessCheck
	}

	return &Handler{
		ready: ready,
	}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	switch r.URL.Path {
	case "/livez":
		writeHealthResponse(w, http.StatusOK)
	case "/readyz":
		if !h.ready() {
			writeHealthResponse(w, http.StatusServiceUnavailable)
			return
		}

		writeHealthResponse(w, http.StatusOK)
	default:
		http.NotFound(w, r)
	}
}

func writeHealthResponse(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte("ok\n"))
}
