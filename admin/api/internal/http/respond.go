// Package http holds the transport helpers every slice's handlers share.
//
// It is imported as httpx, because a package named http alongside net/http reads
// badly at the call site otherwise. It knows nothing about any particular slice.
package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// JSON writes a value as the response body.
//
// A failure to encode is logged and otherwise swallowed: the status line and
// headers are already on the wire by the time encoding runs, so there is no way
// left to tell the client anything different.
func JSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("write response body", slog.Any("error", err))
	}
}

// ErrorBody is the shape every failure answers with, so a caller can branch on
// `error` without matching prose.
type ErrorBody struct {
	// Error is a stable, machine-readable code.
	Error string `json:"error"`
	// Message is a human-readable explanation. It never carries internal detail.
	Message string `json:"message"`
}

// Error writes a failure response.
func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, ErrorBody{Error: code, Message: message})
}

// DecodeJSON reads a JSON request body, answering the caller itself when it
// cannot, and reports whether the handler should continue.
//
// Unknown fields are refused. One is usually a misspelled field, and accepting it
// silently leaves the caller believing it set something it did not.
func DecodeJSON(w http.ResponseWriter, r *http.Request, into any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		Error(w, http.StatusBadRequest, "invalid_request", "the request body is not valid JSON for this endpoint")
		return false
	}
	return true
}
