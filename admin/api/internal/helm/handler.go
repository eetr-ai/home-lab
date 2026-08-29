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
// The scope is named inside each HandleFunc call rather than applied to the mux,
// because it differs per route and only this file knows which is which. Keeping
// the wrapper inside the call also keeps the route literal findable by the
// OpenAPI coverage test, which reads this source rather than the running mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/helm/releases",
		h.guard.Require(auth.ScopeRead, h.listReleases))
	mux.HandleFunc("GET /api/helm/namespaces/{namespace}/releases",
		h.guard.Require(auth.ScopeRead, h.listNamespaceReleases))
	mux.HandleFunc("GET /api/helm/namespaces/{namespace}/releases/{release}",
		h.guard.Require(auth.ScopeRead, h.readRelease))
	mux.HandleFunc("GET /api/helm/namespaces/{namespace}/releases/{release}/history",
		h.guard.Require(auth.ScopeRead, h.readHistory))
	mux.HandleFunc("GET /api/helm/chart-versions",
		h.guard.Require(auth.ScopeRead, h.listChartVersions))
	mux.HandleFunc("POST /api/helm/namespaces/{namespace}/releases/{release}/rollback",
		h.guard.Require(auth.ScopeDeploy, h.rollbackRelease))
	mux.HandleFunc("DELETE /api/helm/namespaces/{namespace}/releases/{release}",
		h.guard.Require(auth.ScopeDeploy, h.uninstallRelease))
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

// listChartVersions returns the versions a chart reference offers.
//
//	@Summary		List the versions a chart reference offers
//	@Description	The reference is an OCI or HTTPS chart URL whose last path segment is the
//	@Description	chart name — oci://ghcr.io/org/charts/podinfo, or
//	@Description	https://stefanprodan.github.io/podinfo/podinfo. The registry is contacted
//	@Description	when this is called, so an unreachable one is a 502 rather than an empty list.
//	@Tags			helm
//	@Produce		json
//	@Security		BearerAuth
//	@Param			ref	query		string	true	"Chart reference"
//	@Success		200	{array}		helm.ChartVersion
//	@Failure		400	{object}	http.ErrorBody
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Failure		502	{object}	http.ErrorBody
//	@Router			/api/helm/chart-versions [get]
func (h *Handler) listChartVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := h.service.ListChartVersions(r.Context(), r.URL.Query().Get("ref"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, versions)
}

// rollbackRelease returns a release to an earlier revision.
//
//	@Summary		Roll a Helm release back to an earlier revision
//	@Description	Accepted, not performed: Helm waits for pods, which outlasts this request.
//	@Description	Read the release to see whether it succeeded. A rollback creates a new
//	@Description	revision rather than restoring the old one, so it can itself be rolled back.
//	@Tags			helm
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	path		string				true	"Namespace name"
//	@Param			release		path		string				true	"Release name"
//	@Param			request		body		helm.RollbackRequest	true	"Revision to roll back to"
//	@Success		202			{object}	helm.Accepted
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		401			{object}	http.ErrorBody
//	@Failure		403			{object}	http.ErrorBody
//	@Failure		404			{object}	http.ErrorBody
//	@Failure		409			{object}	http.ErrorBody
//	@Router			/api/helm/namespaces/{namespace}/releases/{release}/rollback [post]
func (h *Handler) rollbackRelease(w http.ResponseWriter, r *http.Request) {
	var request RollbackRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	accepted, err := h.service.Rollback(r.Context(),
		r.PathValue("namespace"), r.PathValue("release"), request.Revision)
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, accepted)
}

// uninstallRelease removes a release from the cluster.
//
//	@Summary		Uninstall a Helm release
//	@Description	Removes the release and everything it created. Any deployment record this
//	@Description	lab holds is left alone — uninstalling means taking it off the cluster, not
//	@Description	forgetting the values that were written for it.
//	@Tags			helm
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	path		string	true	"Namespace name"
//	@Param			release		path		string	true	"Release name"
//	@Success		202			{object}	helm.Accepted
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		401			{object}	http.ErrorBody
//	@Failure		403			{object}	http.ErrorBody
//	@Failure		404			{object}	http.ErrorBody
//	@Failure		409			{object}	http.ErrorBody
//	@Router			/api/helm/namespaces/{namespace}/releases/{release} [delete]
func (h *Handler) uninstallRelease(w http.ResponseWriter, r *http.Request) {
	accepted, err := h.service.Uninstall(r.Context(),
		r.PathValue("namespace"), r.PathValue("release"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, accepted)
}

// respondError maps this slice's errors to status codes.
func respondError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidName), errors.Is(err, ErrInvalidChartRef),
		errors.Is(err, ErrInvalidValues), errors.Is(err, ErrValuesTooLarge):
		httpx.Error(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrAlreadyExists):
		httpx.Error(w, http.StatusConflict, "already_exists", err.Error())
	case errors.Is(err, ErrInProgress):
		// 409 rather than 429. Nothing is rate-limiting the caller; one operation
		// against this release is already running, and the answer is to wait for
		// it rather than to try again with a delay.
		httpx.Error(w, http.StatusConflict, "in_progress", err.Error())
	case errors.Is(err, ErrRepositoryUnreachable):
		// 502 rather than 400 or 404. The request was well formed and the chart
		// may well exist; something between here and the registry did not answer,
		// and blaming the caller would send them to check a version number that
		// was never the problem.
		httpx.Error(w, http.StatusBadGateway, "repository_unreachable", err.Error())
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
