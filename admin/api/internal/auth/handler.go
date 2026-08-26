package auth

import (
	"net/http"

	httpx "github.com/eetr-ai/home-lab/admin/api/internal/http"
)

// Handler serves the identity endpoints.
type Handler struct{}

// NewHandler builds the handler.
func NewHandler() *Handler {
	return &Handler{}
}

// Register adds the identity routes to a mux. The mux it is given is expected to
// already sit behind Middleware.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/whoami", h.whoami)
}

// WhoamiResponse describes the caller a request authenticated as.
type WhoamiResponse struct {
	// Subject is the identity provider's stable subject claim.
	Subject string `json:"subject"`
	// Email is the caller's email when the token carried the claim.
	Email string `json:"email,omitempty"`
}

// whoami reports who the presented token belongs to.
//
// Small, but not decorative: it is the endpoint that answers "is my token any
// good, and who does this API think I am" without needing a token good enough to
// change something. That is the first thing a new caller — an operator with a
// fresh client, or an agent — needs to know.
//
//	@Summary		Describe the authenticated caller
//	@Description	Reports the subject and email the presented bearer token belongs to.
//	@Tags			identity
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	auth.WhoamiResponse
//	@Failure		401	{object}	http.ErrorBody
//	@Router			/api/whoami [get]
func (h *Handler) whoami(w http.ResponseWriter, r *http.Request) {
	subject, ok := SubjectFrom(r.Context())
	if !ok {
		// Unreachable behind Middleware, and answered rather than asserted: a
		// wiring mistake should surface as a clear 401 rather than a panic that
		// takes the process down.
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "the request carries no verified caller")
		return
	}
	httpx.JSON(w, http.StatusOK, WhoamiResponse{Subject: subject.ID, Email: subject.Email})
}
