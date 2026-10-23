package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/platform/errdefs"
)

var statusByCategory = map[errdefs.Category]int{
	errdefs.Validation:      http.StatusBadRequest,
	errdefs.Unauthenticated: http.StatusUnauthorized,
	errdefs.Forbidden:       http.StatusForbidden,
	errdefs.NotFound:        http.StatusNotFound,
	errdefs.Conflict:        http.StatusConflict,
	errdefs.Internal:        http.StatusInternalServerError,
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// --- input ---

func Decode[T any](r *http.Request) (T, error) {
	var v T
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		return v, errdefs.Validationf("INVALID_JSON", "request body is not valid JSON")
	}
	return v, nil
}

func PathUUID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		return uuid.Nil, errdefs.Validationf("INVALID_ID", "%s must be a uuid", name)
	}
	return id, nil
}

func Query(r *http.Request, names ...string) map[string]string {
	q := r.URL.Query()
	out := make(map[string]string, len(names))
	for _, n := range names {
		out[n] = q.Get(n)
	}
	return out
}

// --- output ---

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func WriteError(w http.ResponseWriter, err error) {
	var e *errdefs.Error
	if !errors.As(err, &e) {
		e = errdefs.Internalf(err, "internal error")
	}
	if e.Category == errdefs.Internal {
		log.Printf("internal error: %v", e.Cause)
	}
	status, ok := statusByCategory[e.Category]
	if !ok {
		status = http.StatusInternalServerError
	}
	WriteJSON(w, status, errorResponse{Code: e.Code, Message: e.Message})
}

func Respond(w http.ResponseWriter, status int, v any, err error) {
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, status, v)
}

func OK(w http.ResponseWriter, v any, err error)      { Respond(w, http.StatusOK, v, err) }
func Created(w http.ResponseWriter, v any, err error) { Respond(w, http.StatusCreated, v, err) }

func NoContent(w http.ResponseWriter, err error) {
	if err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// WithTimeout bounds every request's context, so a stuck dependency (a lock
// wait, a pool wait, a rail call) gives the connection back instead of holding
// it until the client goes away.
func WithTimeout(h http.Handler, d time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), d)
		defer cancel()
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}
