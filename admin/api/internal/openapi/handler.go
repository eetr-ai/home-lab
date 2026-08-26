package openapi

import (
	"net/http"
)

// Handler serves the API description.
type Handler struct{}

// New builds the handler.
func New() *Handler {
	return &Handler{}
}

// Register adds the description route to a mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /openapi.json", h.document)
}

// document returns the OpenAPI description of this API.
//
// Unauthenticated on purpose, and it is the only endpoint that is. The document
// describes shapes, not data: it says an endpoint to list databases exists, never
// which databases there are. A caller needs it before it has done anything, and
// requiring a token to learn how to present a token is a loop.
//
//	@Summary		The OpenAPI description of this API
//	@Description	Returns the generated OpenAPI document describing every endpoint. Requires no authentication.
//	@Tags			meta
//	@Produce		json
//	@Success		200	{object}	object
//	@Router			/openapi.json [get]
func (h *Handler) document(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// Short, not absent: the document changes only when the image does, but a
	// stale copy in a proxy would describe endpoints an upgraded API no longer has.
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(Spec())
}
