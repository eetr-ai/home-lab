package helm

import (
	"context"
	"net/http"
	"strconv"

	httpx "github.com/eetr-ai/home-lab/admin/api/internal/http"
)

// defaultLogTail is how much history a log request starts with.
//
// Enough to see why something failed without replaying a whole chart's hooks. A
// caller that wants more says so.
const defaultLogTail = 500

// maxLogTail bounds what a caller may ask for.
const maxLogTail = 5000

// listJobs returns the Helm operations this panel is running or has run.
//
//	@Summary		List Helm jobs
//	@Description	Every Helm mutation runs as a Kubernetes Job in the panel's own
//	@Description	namespace. This is how a caller that did not start one finds it — a page
//	@Description	loaded after the operation began, or after the panel's own pods were
//	@Description	replaced by the upgrade being watched.
//	@Description
//	@Description	Finished jobs are removed by the cluster after this lab's configured TTL,
//	@Description	so this is a live view rather than a history. What survives is the release
//	@Description	and its deployment record.
//	@Tags			helm
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	query		string	false	"Only jobs targeting this namespace"
//	@Param			release		query		string	false	"Only jobs targeting this release"
//	@Param			deployment	query		string	false	"Only jobs for this deployment"
//	@Success		200			{array}		helm.Job
//	@Failure		401			{object}	http.ErrorBody
//	@Failure		403			{object}	http.ErrorBody
//	@Failure		501			{object}	http.ErrorBody
//	@Router			/api/helm/jobs [get]
func (h *Handler) listJobs(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	jobs, err := h.service.ListJobs(r.Context(), JobFilter{
		Namespace:    query.Get("namespace"),
		Release:      query.Get("release"),
		DeploymentID: query.Get("deployment"),
	})
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, jobs)
}

// readJob returns one Helm operation's status.
//
//	@Summary		Read a Helm job
//	@Description	The status of one operation. `phase` is pending, running, succeeded, or
//	@Description	failed.
//	@Description
//	@Description	A succeeded job is not the same as a successful deploy: with
//	@Description	rollbackOnFailure set, a failed upgrade is undone and the job that undid
//	@Description	it succeeded. The completion rule is still to read the deployment and
//	@Description	assert both its status and its chart version.
//	@Tags			helm
//	@Produce		json
//	@Security		BearerAuth
//	@Param			job	path		string	true	"Job name"
//	@Success		200	{object}	helm.Job
//	@Failure		400	{object}	http.ErrorBody
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Failure		501	{object}	http.ErrorBody
//	@Router			/api/helm/jobs/{job} [get]
func (h *Handler) readJob(w http.ResponseWriter, r *http.Request) {
	job, err := h.service.ReadJob(r.Context(), r.PathValue("job"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, job)
}

// jobLogs streams the log of the pod performing an operation.
//
//	@Summary		Read a Helm job's log
//	@Description	Helm's own account of what it did, as plain text. This is where the reason
//	@Description	for a failed deploy is — the job's status says that it failed, not why.
//	@Description
//	@Description	A 404 with code `no_pod_yet` means the pod has not been scheduled, which
//	@Description	is the normal state for the first moment of every operation: retry. A 404
//	@Description	with `not_found` means the job is gone.
//	@Tags			helm
//	@Produce		plain
//	@Security		BearerAuth
//	@Param			job		path		string	true	"Job name"
//	@Param			follow	query		boolean	false	"Keep the stream open"
//	@Param			tail	query		integer	false	"Lines of history first (default 500, max 5000)"
//	@Success		200		{string}	string	"the log"
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		401		{object}	http.ErrorBody
//	@Failure		403		{object}	http.ErrorBody
//	@Failure		404		{object}	http.ErrorBody
//	@Failure		501		{object}	http.ErrorBody
//	@Router			/api/helm/jobs/{job}/logs [get]
func (h *Handler) jobLogs(w http.ResponseWriter, r *http.Request) {
	if !httpx.ClearWriteDeadline(w, "helm job log") {
		return
	}

	ctx, cancel := contextWithStreamLimit(r)
	defer cancel()

	stream, err := h.service.JobLogs(ctx, r.PathValue("job"),
		queryFlag(r, "follow"), tailLines(r.URL.Query().Get("tail")))
	if err != nil {
		// Nothing has been written yet, so this can still be a status code. It is
		// the last point at which that is true.
		respondError(w, err)
		return
	}
	defer func() { _ = stream.Close() }()

	httpx.StreamText(w, r.WithContext(ctx), stream, "helm job log stream")
}

// queryFlag reads a boolean query parameter, present-means-true.
func queryFlag(r *http.Request, name string) bool {
	return r.URL.Query().Has(name) && r.URL.Query().Get(name) != "false"
}

// tailLines reads the tail parameter and bounds it.
//
// A malformed value becomes the default rather than an error: the parameter is an
// optimization, and refusing the whole request over it would be a worse answer
// than showing a sensible amount of log.
func tailLines(raw string) int64 {
	if raw == "" {
		return defaultLogTail
	}
	lines, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || lines < 1 {
		return defaultLogTail
	}
	if lines > maxLogTail {
		return maxLogTail
	}
	return lines
}

// contextWithStreamLimit bounds one streamed response.
//
// A forgotten browser tab holds a goroutine here and a connection to the cluster
// for as long as it is open, and neither side has any reason to notice.
func contextWithStreamLimit(r *http.Request) (ctx context.Context, cancel context.CancelFunc) {
	return context.WithTimeout(r.Context(), httpx.MaxStreamDuration)
}
