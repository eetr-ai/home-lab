// Package health answers whether the process is up.
package health

import (
	"net/http"

	httpx "github.com/eetr-ai/home-lab/admin/api/internal/http"
)

// Handler serves the probe endpoint.
type Handler struct{}

// New builds the handler.
func New() *Handler {
	return &Handler{}
}

// Register adds the health routes to a mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", h.healthz)
}

// Response is what the probe answers with.
type Response struct {
	Status string `json:"status"`
}

// healthz reports process liveness.
//
// It deliberately checks nothing downstream. A probe that failed when PostgreSQL
// was unreachable would take the panel out of service at exactly the moment an
// operator needs it to say so — and Kubernetes would restart a process that is
// working perfectly well.
//
//	@Summary		Liveness and readiness probe
//	@Description	Reports whether the process is running. Checks nothing downstream, so it stays
//	@Description	true while a managed database is unreachable.
//	@Tags			health
//	@Produce		json
//	@Success		200	{object}	health.Response
//	@Router			/healthz [get]
func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, Response{Status: "ok"})
}
