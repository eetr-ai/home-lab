package helm

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/eetr-ai/home-lab/admin/api/internal/auth"
	httpx "github.com/eetr-ai/home-lab/admin/api/internal/http"
)

// Handler exposes the Helm slice over HTTP.
type Handler struct {
	service *Service
	guard   *auth.Guard
}

// NewHandler builds the handler.
func NewHandler(service *Service, guard *auth.Guard) *Handler {
	return &Handler{service: service, guard: guard}
}

// Register adds the Helm routes to a mux that already requires a verified caller.
//
// All reads, for now. Every one of them is behind admin:read, which a token
// naming no scopes still satisfies — see auth.Guard.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/helm/releases",
		h.guard.Require(auth.ScopeRead, h.listReleases))
	mux.HandleFunc("GET /api/helm/namespaces/{namespace}/releases",
		h.guard.Require(auth.ScopeRead, h.listNamespaceReleases))
	mux.HandleFunc("GET /api/helm/namespaces/{namespace}/releases/{release}",
		h.guard.Require(auth.ScopeRead, h.readRelease))
	mux.HandleFunc("GET /api/helm/namespaces/{namespace}/releases/{release}/history",
		h.guard.Require(auth.ScopeRead, h.readHistory))
}

// listReleases returns every release in every managed namespace.
//
//	@Summary		List Helm releases
//	@Description	Across every namespace this lab has made a Helm target. A release
//	@Description	installed outside the panel is listed too: Helm's own storage is the
//	@Description	source of truth, so anything it recorded is visible here.
//	@Tags			helm
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{array}		helm.Release
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Failure		501	{object}	http.ErrorBody
//	@Router			/api/helm/releases [get]
func (h *Handler) listReleases(w http.ResponseWriter, r *http.Request) {
	releases, err := h.service.ListReleases(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, releases)
}

// listNamespaceReleases returns the releases in one namespace.
//
//	@Summary		List the Helm releases in one namespace
//	@Tags			helm
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	path		string	true	"Namespace name"
//	@Success		200			{array}		helm.Release
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		401			{object}	http.ErrorBody
//	@Failure		403			{object}	http.ErrorBody
//	@Router			/api/helm/namespaces/{namespace}/releases [get]
func (h *Handler) listNamespaceReleases(w http.ResponseWriter, r *http.Request) {
	releases, err := h.service.ListNamespaceReleases(r.Context(), r.PathValue("namespace"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, releases)
}

// readRelease returns one release with the values it was configured with.
//
//	@Summary		Read a Helm release
//	@Description	The values are the ones supplied at install or upgrade, not the chart's
//	@Description	defaults merged with them. Re-submitting the merged set would pin every
//	@Description	default the chart ships with.
//	@Tags			helm
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	path		string	true	"Namespace name"
//	@Param			release		path		string	true	"Release name"
//	@Success		200			{object}	helm.ReleaseDetail
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		401			{object}	http.ErrorBody
//	@Failure		403			{object}	http.ErrorBody
//	@Failure		404			{object}	http.ErrorBody
//	@Router			/api/helm/namespaces/{namespace}/releases/{release} [get]
func (h *Handler) readRelease(w http.ResponseWriter, r *http.Request) {
	release, err := h.service.ReadRelease(r.Context(),
		r.PathValue("namespace"), r.PathValue("release"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, release)
}

// readHistory returns a release's revisions, newest first.
//
//	@Summary		Read a Helm release's history
//	@Description	Newest revision first. Revisions count up and never repeat: rolling back
//	@Description	to revision 2 creates a new revision rather than returning to that one.
//	@Tags			helm
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	path		string	true	"Namespace name"
//	@Param			release		path		string	true	"Release name"
//	@Success		200			{array}		helm.Revision
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		401			{object}	http.ErrorBody
//	@Failure		403			{object}	http.ErrorBody
//	@Failure		404			{object}	http.ErrorBody
//	@Router			/api/helm/namespaces/{namespace}/releases/{release}/history [get]
func (h *Handler) readHistory(w http.ResponseWriter, r *http.Request) {
	revisions, err := h.service.ReadHistory(r.Context(),
		r.PathValue("namespace"), r.PathValue("release"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, revisions)
}

// respondError maps this slice's errors to status codes.
func respondError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidName):
		httpx.Error(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrProtected), errors.Is(err, ErrUnmanaged):
		// 403 rather than 404. The namespace exists and is readable elsewhere in
		// this panel; what is refused is Helm's reach into it, and saying so is
		// what tells an operator to edit a values file rather than to go looking
		// for a release that was never missing.
		httpx.Error(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "forbidden",
			"the panel's service account is not permitted to read this namespace's releases")
	case errors.Is(err, ErrNotConfigured):
		// 501 rather than 404. The capability is built and served; this lab has
		// not named a namespace for it to work in. A 404 would read as "no such
		// endpoint" and send someone looking for a missing route.
		httpx.Error(w, http.StatusNotImplemented, "not_configured",
			"no namespaces are configured for Helm")
	default:
		slog.Error("helm request failed", slog.Any("error", err))
		httpx.Error(w, http.StatusInternalServerError, "internal_error",
			"the request could not be completed")
	}
}
