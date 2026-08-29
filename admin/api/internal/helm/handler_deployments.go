package helm

import (
	"net/http"

	"github.com/eetr-ai/home-lab/admin/api/internal/auth"
	httpx "github.com/eetr-ai/home-lab/admin/api/internal/http"
)

// listDeployments returns the charts this lab has declared.
//
//	@Summary		List declared Helm deployments
//	@Description	Each carries the newest declared version and how it stands against the
//	@Description	cluster: in-sync, pending, drifted, not-installed, or unknown when the
//	@Description	live release could not be read.
//	@Tags			helm
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	query		string	false	"Only this namespace"
//	@Success		200			{array}		helm.DeploymentSummary
//	@Failure		401			{object}	http.ErrorBody
//	@Failure		403			{object}	http.ErrorBody
//	@Failure		501			{object}	http.ErrorBody
//	@Failure		503			{object}	http.ErrorBody
//	@Router			/api/helm/deployments [get]
func (h *Handler) listDeployments(w http.ResponseWriter, r *http.Request) {
	deployments, err := h.service.ListDeployments(r.Context(), r.URL.Query().Get("namespace"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, deployments)
}

// declareDeployment records a chart for a namespace without deploying it.
//
//	@Summary		Declare a Helm deployment
//	@Description	Writes the record and its first version. Nothing reaches the cluster until
//	@Description	a rollout, so a half-written values file is a saved draft rather than a
//	@Description	failed install.
//	@Tags			helm
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		helm.DeclareRequest	true	"The chart, namespace and first values"
//	@Success		201		{object}	helm.Deployment
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		401		{object}	http.ErrorBody
//	@Failure		403		{object}	http.ErrorBody
//	@Failure		409		{object}	http.ErrorBody
//	@Failure		501		{object}	http.ErrorBody
//	@Failure		503		{object}	http.ErrorBody
//	@Router			/api/helm/deployments [post]
func (h *Handler) declareDeployment(w http.ResponseWriter, r *http.Request) {
	var request DeclareRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	deployment, err := h.service.Declare(r.Context(), request, actorFrom(r))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, deployment)
}

// readDeployment returns one deployment with its versions and its live release.
//
//	@Summary		Read a declared Helm deployment
//	@Tags			helm
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Deployment id"
//	@Success		200	{object}	helm.DeploymentDetail
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Failure		501	{object}	http.ErrorBody
//	@Failure		503	{object}	http.ErrorBody
//	@Router			/api/helm/deployments/{id} [get]
func (h *Handler) readDeployment(w http.ResponseWriter, r *http.Request) {
	deployment, err := h.service.ReadDeployment(r.Context(), r.PathValue("id"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, deployment)
}

// forgetDeployment removes the record and leaves the release alone.
//
//	@Summary		Forget a declared Helm deployment
//	@Description	Removes this lab's record and every version of it. The release on the
//	@Description	cluster is untouched — uninstalling it is a separate request.
//	@Tags			helm
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path	string	true	"Deployment id"
//	@Success		204	"forgotten"
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Failure		501	{object}	http.ErrorBody
//	@Failure		503	{object}	http.ErrorBody
//	@Router			/api/helm/deployments/{id} [delete]
func (h *Handler) forgetDeployment(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Forget(r.Context(), r.PathValue("id")); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listDeploymentVersions returns every declared version, newest first.
//
//	@Summary		List a deployment's declared versions
//	@Description	Append-only: editing values writes a new version rather than changing one,
//	@Description	so this doubles as the record of who changed what and when.
//	@Tags			helm
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Deployment id"
//	@Success		200	{array}		helm.DeploymentVersion
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Failure		501	{object}	http.ErrorBody
//	@Failure		503	{object}	http.ErrorBody
//	@Router			/api/helm/deployments/{id}/versions [get]
func (h *Handler) listDeploymentVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := h.service.ListVersions(r.Context(), r.PathValue("id"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, versions)
}

// addDeploymentVersion declares another version without rolling it out.
//
//	@Summary		Add a version to a Helm deployment
//	@Tags			helm
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string				true	"Deployment id"
//	@Param			request	body		helm.VersionRequest	true	"Chart version and values"
//	@Success		201		{object}	helm.DeploymentVersion
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		401		{object}	http.ErrorBody
//	@Failure		403		{object}	http.ErrorBody
//	@Failure		404		{object}	http.ErrorBody
//	@Failure		501		{object}	http.ErrorBody
//	@Failure		503		{object}	http.ErrorBody
//	@Router			/api/helm/deployments/{id}/versions [post]
func (h *Handler) addDeploymentVersion(w http.ResponseWriter, r *http.Request) {
	var request VersionRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	version, err := h.service.AddVersion(r.Context(), r.PathValue("id"), request, actorFrom(r))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, version)
}

// rolloutDeployment applies a declared version to the cluster.
//
//	@Summary		Roll a declared version out to the cluster
//	@Description	Accepted, not performed: Helm waits for pods, which outlasts this request.
//	@Description	Whether this installs or upgrades is decided by what Helm already has.
//	@Description	Read the deployment to see whether it succeeded — it is done when the
//	@Description	status is no longer pending AND the chart version matches what was asked for.
//	@Tags			helm
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string				true	"Deployment id"
//	@Param			request	body		helm.RolloutRequest	false	"Which version, and how to fail"
//	@Success		202		{object}	helm.Accepted
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		401		{object}	http.ErrorBody
//	@Failure		403		{object}	http.ErrorBody
//	@Failure		404		{object}	http.ErrorBody
//	@Failure		409		{object}	http.ErrorBody
//	@Failure		501		{object}	http.ErrorBody
//	@Failure		503		{object}	http.ErrorBody
//	@Router			/api/helm/deployments/{id}/rollout [post]
func (h *Handler) rolloutDeployment(w http.ResponseWriter, r *http.Request) {
	var request RolloutRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	accepted, err := h.service.Rollout(r.Context(), r.PathValue("id"), request)
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, accepted)
}

// pipelineRollout is the endpoint a pipeline calls.
//
//	@Summary		Deploy a new chart version from a pipeline
//	@Description	Takes a chart version and optional values overrides, merges the overrides
//	@Description	over the newest declared values, stores the result as a new version, and
//	@Description	rolls it out. Omitting values carries the previous ones forward unchanged,
//	@Description	comments included, so a pipeline that owns only a chart tag cannot erase an
//	@Description	operator's configuration.
//	@Description
//	@Description	Accepted, not performed. Poll the deployment until its status is no longer
//	@Description	pending, then assert BOTH that the status is deployed AND that the chart
//	@Description	version equals what was asked for: with rollbackOnFailure set, a failed
//	@Description	deploy is undone and lands "deployed" on the previous version.
//	@Tags			helm
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string					true	"Deployment id"
//	@Param			request	body		helm.PipelineRequest	true	"Chart version and value overrides"
//	@Success		202		{object}	helm.Accepted
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		401		{object}	http.ErrorBody
//	@Failure		403		{object}	http.ErrorBody
//	@Failure		404		{object}	http.ErrorBody
//	@Failure		409		{object}	http.ErrorBody
//	@Failure		501		{object}	http.ErrorBody
//	@Failure		503		{object}	http.ErrorBody
//	@Router			/api/helm/deployments/{id} [put]
func (h *Handler) pipelineRollout(w http.ResponseWriter, r *http.Request) {
	var request PipelineRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	accepted, err := h.service.PipelineRollout(r.Context(), r.PathValue("id"), request, actorFrom(r))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, accepted)
}

// actorFrom names whoever is making the change, for the record.
//
// The email for a person, and the client id for a pipeline — which is the only
// identity a client_credentials token carries here, because this provider leaves
// `sub` empty for one. Never empty: an unattributed version in the history is
// the one nobody can explain later.
func actorFrom(r *http.Request) string {
	subject, ok := auth.SubjectFrom(r.Context())
	if !ok {
		return "unknown"
	}

	for _, candidate := range []string{subject.Email, subject.ClientID, subject.ID} {
		if candidate != "" {
			return candidate
		}
	}
	return "unknown"
}
