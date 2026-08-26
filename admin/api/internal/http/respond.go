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
