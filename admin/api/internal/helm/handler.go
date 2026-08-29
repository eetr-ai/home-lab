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
// Reads take admin:read; everything that changes the cluster takes admin:deploy
// rather than admin:write. The separation is the point: admin:deploy is the scope
// a pipeline holds, and a pipeline should be able to roll a release forward
// without also being able to scale a workload to zero.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/helm/charts",
		h.guard.Require(auth.ScopeRead, h.listCharts))
	mux.HandleFunc("GET /api/helm/charts/{chart}/versions",
		h.guard.Require(auth.ScopeRead, h.listChartVersions))
	mux.HandleFunc("GET /api/helm/releases",
		h.guard.Require(auth.ScopeRead, h.listReleases))
	mux.HandleFunc("GET /api/helm/namespaces/{namespace}/releases",
		h.guard.Require(auth.ScopeRead, h.listNamespaceReleases))
	mux.HandleFunc("GET /api/helm/namespaces/{namespace}/releases/{release}",
		h.guard.Require(auth.ScopeRead, h.readRelease))
	mux.HandleFunc("GET /api/helm/namespaces/{namespace}/releases/{release}/history",
		h.guard.Require(auth.ScopeRead, h.readHistory))

	mux.HandleFunc("POST /api/helm/namespaces/{namespace}/releases",
		h.guard.Require(auth.ScopeDeploy, h.installRelease))
	mux.HandleFunc("PUT /api/helm/namespaces/{namespace}/releases/{release}",
		h.guard.Require(auth.ScopeDeploy, h.upgradeRelease))
	mux.HandleFunc("DELETE /api/helm/namespaces/{namespace}/releases/{release}",
		h.guard.Require(auth.ScopeDeploy, h.uninstallRelease))
	mux.HandleFunc("POST /api/helm/namespaces/{namespace}/releases/{release}/rollback",
		h.guard.Require(auth.ScopeDeploy, h.rollbackRelease))
}

// listCharts returns the charts this lab will install.
//
//	@Summary		List the chart catalog
//	@Description	The catalog is configuration, not discovery: a request names an entry
//	@Description	from this list and never a URL, which is what bounds what can be
//	@Description	installed. Each entry's versions are read from its repository and cached
//	@Description	briefly; when the repository cannot be reached the entry is still
//	@Description	returned, marked unavailable, carrying whatever versions configuration
//	@Description	pinned.
//	@Tags			helm
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{array}		helm.ChartListing
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Failure		501	{object}	http.ErrorBody
//	@Router			/api/helm/charts [get]
func (h *Handler) listCharts(w http.ResponseWriter, r *http.Request) {
	charts, err := h.service.ListCharts(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, charts)
}

// listChartVersions returns one catalogue entry's installable versions.
//
//	@Summary		List a chart's installable versions
//	@Tags			helm
//	@Produce		json
//	@Security		BearerAuth
//	@Param			chart	path		string	true	"Catalog entry name"
//	@Success		200		{object}	helm.ChartListing
//	@Failure		401		{object}	http.ErrorBody
//	@Failure		403		{object}	http.ErrorBody
//	@Failure		404		{object}	http.ErrorBody
//	@Failure		501		{object}	http.ErrorBody
//	@Router			/api/helm/charts/{chart}/versions [get]
func (h *Handler) listChartVersions(w http.ResponseWriter, r *http.Request) {
	listing, err := h.service.ListChartVersions(r.Context(), r.PathValue("chart"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, listing)
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

// installRelease puts a new release on the cluster.
//
//	@Summary		Install a Helm release
//	@Description	Answers 202: Helm waits for pods, which outlasts this request. Every rule
//	@Description	is checked before the 202, so a bad request is a 400 rather than a 202
//	@Description	that quietly fails. Read the release to see what happened — it is
//	@Description	pending-install until it is deployed or failed.
//	@Description	The chart must be a catalog entry and the version exact; a range or
//	@Description	"latest" is refused rather than resolved.
//	@Tags			helm
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	path		string					true	"Namespace name"
//	@Param			request		body		helm.InstallRequest		true	"The release to install"
//	@Success		202			{object}	helm.Accepted
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		401			{object}	http.ErrorBody
//	@Failure		403			{object}	http.ErrorBody
//	@Failure		404			{object}	http.ErrorBody
//	@Failure		409			{object}	http.ErrorBody
//	@Failure		501			{object}	http.ErrorBody
//	@Router			/api/helm/namespaces/{namespace}/releases [post]
func (h *Handler) installRelease(w http.ResponseWriter, r *http.Request) {
	var request InstallRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}
	request.Namespace = r.PathValue("namespace")

	accepted, err := h.service.Install(r.Context(), request)
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, accepted)
}

// upgradeRelease moves a release to another version.
//
//	@Summary		Upgrade a Helm release
//	@Description	The endpoint a pipeline calls. The body normally carries only a version:
//	@Description	absent values mean the release keeps its own, so a pipeline that owns an
//	@Description	image tag need not know the rest of the configuration and cannot erase it.
//	@Description	Which chart the release came from is read from Helm's storage, not taken
//	@Description	from the request, so an upgrade cannot replace a release with something
//	@Description	else.
//	@Description	Answers 202. A pipeline's completion check is not "the status became
//	@Description	terminal" but "the status is deployed AND chartVersion is the version I
//	@Description	asked for" — with rollbackOnFailure set, a failed upgrade ends up deployed
//	@Description	on an earlier chart.
//	@Tags			helm
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	path		string					true	"Namespace name"
//	@Param			release		path		string					true	"Release name"
//	@Param			request		body		helm.UpgradeRequest		true	"The version to move to"
//	@Success		202			{object}	helm.Accepted
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		401			{object}	http.ErrorBody
//	@Failure		403			{object}	http.ErrorBody
//	@Failure		404			{object}	http.ErrorBody
//	@Failure		409			{object}	http.ErrorBody
//	@Failure		501			{object}	http.ErrorBody
//	@Router			/api/helm/namespaces/{namespace}/releases/{release} [put]
func (h *Handler) upgradeRelease(w http.ResponseWriter, r *http.Request) {
	var request UpgradeRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}
	request.Namespace = r.PathValue("namespace")
	request.Name = r.PathValue("release")

	accepted, err := h.service.Upgrade(r.Context(), request)
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, accepted)
}

// uninstallRelease removes a release and everything it created.
//
//	@Summary		Uninstall a Helm release
//	@Description	Answers 202. Removes every resource the chart created, which for a chart
//	@Description	with a volume claim includes the claim.
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

// rollbackRelease returns a release to an earlier revision.
//
//	@Summary		Roll a Helm release back
//	@Description	Answers 202. The revision is required rather than defaulted: Helm's own
//	@Description	default is "the previous revision", which is a different operation from
//	@Description	the one an operator clicked in a table of revisions.
//	@Description	Rolling back creates a new revision rather than restoring the old one, so
//	@Description	a rollback can itself be rolled back. It is also permitted from a pending
//	@Description	state, which is how a release wedged by a killed pod is recovered.
//	@Tags			helm
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	path		string					true	"Namespace name"
//	@Param			release		path		string					true	"Release name"
//	@Param			request		body		helm.RollbackRequest	true	"The revision to return to"
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

// The error codes this slice answers with. Named because several conditions map
// to the same one, and two spellings of "forbidden" would be two things for a
// client to switch on.
const (
	codeInvalidRequest = "invalid_request"
	codeNotFound       = "not_found"
	codeForbidden      = "forbidden"
	codeConflict       = "conflict"
)

// statusFor maps this slice's errors to a status and an error code.
//
// A table rather than a switch because there are enough of them now that a
// switch reads as a wall, and because the mapping is data: which condition is a
// 400 and which is a 409 is a decision worth seeing all at once. Order matters
// only in that the first match wins, and no error here wraps another.
var statusFor = []struct {
	err    error
	status int
	code   string
}{
	{ErrInvalidName, http.StatusBadRequest, codeInvalidRequest},
	{ErrValuesTooLarge, http.StatusBadRequest, codeInvalidRequest},
	{ErrUnknownVersion, http.StatusBadRequest, codeInvalidRequest},

	// 404 for a chart that is not catalogued: the caller asked for a thing by
	// name and this lab does not have it, which is the same shape of answer as a
	// missing release even though the reason is an allowlist rather than absence.
	{ErrUnknownChart, http.StatusNotFound, codeNotFound},
	{ErrNotFound, http.StatusNotFound, codeNotFound},

	// 403 rather than 404 for a namespace out of reach. It exists and is readable
	// elsewhere in this panel; what is refused is Helm's reach into it, and
	// saying so is what tells an operator to edit a values file rather than to go
	// looking for a release that was never missing.
	{ErrProtected, http.StatusForbidden, codeForbidden},
	{ErrUnmanaged, http.StatusForbidden, codeForbidden},
	{ErrForbidden, http.StatusForbidden, codeForbidden},

	{ErrAlreadyExists, http.StatusConflict, codeConflict},
	// 409 rather than 429: this is not rate limiting and retrying in a second
	// will not help. Something is already changing this release.
	{ErrInProgress, http.StatusConflict, codeConflict},

	// 502, because the failure is somebody else's. Listing the catalog degrades
	// to what configuration knows; installing does not, because installing
	// something nothing could confirm is worse than not installing it.
	{ErrRepositoryUnreachable, http.StatusBadGateway, "upstream_error"},

	// 501 rather than 404. The capability is built and served; this lab has not
	// switched it on. A 404 would read as "no such endpoint".
	{ErrNotConfigured, http.StatusNotImplemented, "not_configured"},
}

// respondError answers a failed request.
func respondError(w http.ResponseWriter, err error) {
	for _, mapping := range statusFor {
		if errors.Is(err, mapping.err) {
			httpx.Error(w, mapping.status, mapping.code, err.Error())
			return
		}
	}

	slog.Error("helm request failed", slog.Any("error", err))
	httpx.Error(w, http.StatusInternalServerError, "internal_error",
		"the request could not be completed")
}
