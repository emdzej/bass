// Package httpx provides small HTTP helpers used across bass handlers.
package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// WriteJSON writes v as JSON with the given status. Encoding errors are
// logged but not surfaced — the response is already partially flushed at
// that point.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode failed", "err", err)
	}
}

// Error writes a JSON error envelope.
type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func Error(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, errorBody{Error: code, Message: message})
}

// DecodeJSON parses the request body. Returns false (and writes a 400) if
// parsing fails; the caller can simply return.
func DecodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		Error(w, http.StatusBadRequest, "invalid_request", err.Error())
		return false
	}
	return true
}
