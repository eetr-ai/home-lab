package kube

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	httpx "github.com/eetr-ai/home-lab/admin/api/internal/http"
)

// Handler exposes the Kubernetes slice over HTTP.
type Handler struct {
	service *Service
}

// NewHandler builds the handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register adds the Kubernetes routes to a mux that already requires a verified
// caller.
//
// Almost all of it is GETs. Restart and scale change how many pods a workload
// runs and when they last started, and nothing else. The Secret route is the one
// that brings something into existence; see the note on Service for why it is
// here at all.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/kubernetes/namespaces", h.listNamespaces)
	mux.HandleFunc("GET /api/kubernetes/namespaces/{namespace}", h.readNamespace)
	mux.HandleFunc("POST /api/kubernetes/namespaces",
		h.createNamespace)
	mux.HandleFunc("DELETE /api/kubernetes/namespaces/{namespace}",
		h.deleteNamespace)
	mux.HandleFunc("GET /api/kubernetes/namespaces/{namespace}/workloads", h.listWorkloads)
	mux.HandleFunc("GET /api/kubernetes/namespaces/{namespace}/pods", h.listPods)
	mux.HandleFunc("GET /api/kubernetes/namespaces/{namespace}/events", h.listEvents)
	mux.HandleFunc("GET /api/kubernetes/nodes", h.listNodes)
	mux.HandleFunc("GET /api/kubernetes/storage", h.readStorage)
	mux.HandleFunc("GET /api/kubernetes/summary", h.readSummary)
	mux.HandleFunc("GET /api/kubernetes/namespaces/{namespace}/pods/{pod}/logs", h.podLogs)
	mux.HandleFunc("GET /api/kubernetes/namespaces/{namespace}/workloads/{kind}/{name}",
		h.readWorkload)
	mux.HandleFunc("POST /api/kubernetes/namespaces/{namespace}/workloads/{kind}/{name}/restart",
		h.restartWorkload)
	mux.HandleFunc("PUT /api/kubernetes/namespaces/{namespace}/workloads/{kind}/{name}/scale",
		h.scaleWorkload)
	mux.HandleFunc("PUT /api/kubernetes/namespaces/{namespace}/secrets/{name}",
		h.putSecret)
	mux.HandleFunc("POST /api/kubernetes/namespaces/{namespace}/helm-enrolment",
		h.enrolNamespace)
	mux.HandleFunc("DELETE /api/kubernetes/namespaces/{namespace}/helm-enrolment",
		h.revokeNamespace)
}

// listNamespaces returns every namespace in the cluster.
//
//	@Summary		List namespaces
//	@Tags			kubernetes
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{array}		kube.Namespace
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Router			/api/kubernetes/namespaces [get]
func (h *Handler) listNamespaces(w http.ResponseWriter, r *http.Request) {
	namespaces, err := h.service.ListNamespaces(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, namespaces)
}

// readNamespace returns one namespace and whether the panel may delete it.
//
//	@Summary		Read a namespace
//	@Tags			kubernetes
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	path		string	true	"Namespace name"
//	@Success		200			{object}	kube.Namespace
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		401			{object}	http.ErrorBody
//	@Failure		403			{object}	http.ErrorBody
//	@Failure		404			{object}	http.ErrorBody
//	@Router			/api/kubernetes/namespaces/{namespace} [get]
func (h *Handler) readNamespace(w http.ResponseWriter, r *http.Request) {
	namespace, err := h.service.ReadNamespace(r.Context(), r.PathValue("namespace"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, namespace)
}

// createNamespace creates a namespace.
//
//	@Summary		Create a namespace
//	@Description	The panel applies its own labels over any the request supplies: the
//	@Description	Pod Security enforcement level, who manages it, and the marker that
//	@Description	makes it a candidate for Helm. A request may not set a label under
//	@Description	kubernetes.io or k8s.io, since one of those decides whether a pod may
//	@Description	run privileged.
//	@Tags			kubernetes
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		kube.NamespaceSpec	true	"The namespace to create"
//	@Success		201		{object}	kube.Namespace
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		401		{object}	http.ErrorBody
//	@Failure		403		{object}	http.ErrorBody
//	@Failure		409		{object}	http.ErrorBody
//	@Router			/api/kubernetes/namespaces [post]
func (h *Handler) createNamespace(w http.ResponseWriter, r *http.Request) {
	var spec NamespaceSpec
	if !httpx.DecodeJSON(w, r, &spec) {
		return
	}

	namespace, err := h.service.CreateNamespace(r.Context(), spec)
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, namespace)
}

// deleteNamespace deletes a namespace and everything in it.
//
//	@Summary		Delete a namespace
//	@Description	Refused for a protected namespace, and for one that still runs
//	@Description	workloads unless force=true. Deletion cascades to everything in the
//	@Description	namespace and is asynchronous: a 204 means the deletion was accepted,
//	@Description	and the namespace stays in the listing as Terminating until it finishes.
//	@Tags			kubernetes
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	path	string	true	"Namespace name"
//	@Param			force		query	boolean	false	"Delete even though it still runs workloads"
//	@Success		204
//	@Failure		400	{object}	http.ErrorBody
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Failure		409	{object}	http.ErrorBody
//	@Router			/api/kubernetes/namespaces/{namespace} [delete]
func (h *Handler) deleteNamespace(w http.ResponseWriter, r *http.Request) {
	// Only an explicit "true" forces. Anything else — absent, empty, "1", a typo —
	// is not the deliberate act this guard exists to require.
	force := r.URL.Query().Get("force") == "true"

	err := h.service.DeleteNamespace(r.Context(), r.PathValue("namespace"), force)
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusNoContent, nil)
}

// listWorkloads returns the controllers running pods in one namespace.
//
//	@Summary		List workloads
//	@Description	Deployments, StatefulSets, and DaemonSets as one list. The question is
//	@Description	what is running, not which kind of controller runs it.
//	@Tags			kubernetes
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	path		string	true	"Namespace"
//	@Success		200			{array}		kube.Workload
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		401			{object}	http.ErrorBody
//	@Failure		403			{object}	http.ErrorBody
//	@Router			/api/kubernetes/namespaces/{namespace}/workloads [get]
func (h *Handler) listWorkloads(w http.ResponseWriter, r *http.Request) {
	workloads, err := h.service.ListWorkloads(r.Context(), r.PathValue("namespace"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, workloads)
}

// listPods returns the pods in one namespace.
//
//	@Summary		List pods
//	@Description	Reports a summary alongside the phase. The phase alone does not say that a
//	@Description	Running pod is serving nothing, or that one is being deleted.
//	@Tags			kubernetes
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	path		string	true	"Namespace"
//	@Success		200			{array}		kube.Pod
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		401			{object}	http.ErrorBody
//	@Failure		403			{object}	http.ErrorBody
//	@Router			/api/kubernetes/namespaces/{namespace}/pods [get]
func (h *Handler) listPods(w http.ResponseWriter, r *http.Request) {
	pods, err := h.service.ListPods(r.Context(), r.PathValue("namespace"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, pods)
}

// listEvents returns the recent events in one namespace, most recent first.
//
//	@Summary		List events
//	@Description	Most recent first, capped at 100. Events carry the reason a pod is stuck
//	@Description	when the pod itself only reports the symptom.
//	@Tags			kubernetes
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	path		string	true	"Namespace"
//	@Success		200			{array}		kube.Event
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		401			{object}	http.ErrorBody
//	@Failure		403			{object}	http.ErrorBody
//	@Router			/api/kubernetes/namespaces/{namespace}/events [get]
func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.service.ListEvents(r.Context(), r.PathValue("namespace"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, events)
}

// listNodes returns every machine in the cluster.
//
//	@Summary		List nodes
//	@Description	Capacity, what pods have reserved against it, and — when metrics-server
//	@Description	is installed — what is actually being used. A node whose usage is absent
//	@Description	reports no reading rather than zero.
//	@Tags			kubernetes
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{array}		kube.Node
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Router			/api/kubernetes/nodes [get]
func (h *Handler) listNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.service.ListNodes(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, nodes)
}

// readStorage returns the cluster's persistent storage.
//
//	@Summary		Read storage
//	@Description	Persistent volume claims and the volumes behind them. Both, because a
//	@Description	released volume still holding data appears in neither list alone.
//	@Tags			kubernetes
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	kube.Storage
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Router			/api/kubernetes/storage [get]
func (h *Handler) readStorage(w http.ResponseWriter, r *http.Request) {
	storage, err := h.service.ReadStorage(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, storage)
}

// readSummary returns the overview dashboard's rollup.
//
//	@Summary		Read cluster summary
//	@Description	Counts and totals across nodes, pods, workloads, and storage, in one call
//	@Description	so the dashboard neither renders nor fails in pieces.
//	@Description
//	@Description	nodes.usage is a sum over the nodes that reported a reading, so it is zero
//	@Description	both for an idle cluster and for one nothing measured. metricsAvailable is
//	@Description	what tells those apart: when it is false, treat nodes.usage as absent
//	@Description	rather than as a measurement of zero.
//	@Tags			kubernetes
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	kube.Summary
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Router			/api/kubernetes/summary [get]
func (h *Handler) readSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.service.ReadSummary(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, summary)
}

// readWorkload returns one workload and everything around it.
//
//	@Summary		Read a workload
//	@Description	The workload with its pods, the services that reach them, the volume
//	@Description	claims they mount, and the events about any of it — in one call, because
//	@Description	the pieces are found by following the workload's own selector and a
//	@Description	client doing that would need to know how Kubernetes labels relate.
//	@Tags			kubernetes
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	path		string	true	"Namespace"
//	@Param			kind		path		string	true	"Deployment, StatefulSet, or DaemonSet"
//	@Param			name		path		string	true	"Workload name"
//	@Success		200			{object}	kube.WorkloadDetail
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		401			{object}	http.ErrorBody
//	@Failure		403			{object}	http.ErrorBody
//	@Failure		404			{object}	http.ErrorBody
//	@Router			/api/kubernetes/namespaces/{namespace}/workloads/{kind}/{name} [get]
func (h *Handler) readWorkload(w http.ResponseWriter, r *http.Request) {
	detail, err := h.service.ReadWorkload(r.Context(),
		r.PathValue("kind"), r.PathValue("namespace"), r.PathValue("name"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, detail)
}

// restartWorkload rolls a workload's pods.
//
//	@Summary		Restart a workload
//	@Description	Stamps the pod template's restartedAt annotation, the same mechanism
//	@Description	`kubectl rollout restart` uses, so the controller replaces its pods under
//	@Description	its own rollout strategy. Nothing is deleted. Deployments and
//	@Description	StatefulSets only.
//	@Tags			kubernetes
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	path	string	true	"Namespace"
//	@Param			kind		path	string	true	"Deployment or StatefulSet"
//	@Param			name		path	string	true	"Workload name"
//	@Success		204
//	@Failure		400	{object}	http.ErrorBody
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Router			/api/kubernetes/namespaces/{namespace}/workloads/{kind}/{name}/restart [post]
func (h *Handler) restartWorkload(w http.ResponseWriter, r *http.Request) {
	err := h.service.RestartWorkload(r.Context(),
		r.PathValue("kind"), r.PathValue("namespace"), r.PathValue("name"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusNoContent, nil)
}

// scaleWorkload sets a workload's replica count.
//
//	@Summary		Scale a workload
//	@Description	Sets the desired replica count through the scale subresource. Deployments
//	@Description	and StatefulSets only: a DaemonSet's count comes from how many nodes it
//	@Description	matches, so there is nothing to set.
//	@Tags			kubernetes
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	path	string				true	"Namespace"
//	@Param			kind		path	string				true	"Deployment or StatefulSet"
//	@Param			name		path	string				true	"Workload name"
//	@Param			request		body	kube.ScaleRequest	true	"Desired replicas"
//	@Success		204
//	@Failure		400	{object}	http.ErrorBody
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Failure		409	{object}	http.ErrorBody
//	@Router			/api/kubernetes/namespaces/{namespace}/workloads/{kind}/{name}/scale [put]
func (h *Handler) scaleWorkload(w http.ResponseWriter, r *http.Request) {
	var request ScaleRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}
	if request.Replicas == nil {
		// Refused rather than defaulted. Zero is a real replica count, so treating
		// a missing one as zero would let `{}` take a workload down.
		httpx.Error(w, http.StatusBadRequest, "invalid_request",
			"replicas is required; it is the count to scale to")
		return
	}

	err := h.service.ScaleWorkload(r.Context(),
		r.PathValue("kind"), r.PathValue("namespace"), r.PathValue("name"), *request.Replicas)
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusNoContent, nil)
}

// podLogs streams one pod's log.
//
//	@Summary		Stream a pod's log
//	@Description	Plain text, chunked, flushed per line. With follow=true the response stays
//	@Description	open and new lines arrive as they are written; closing the connection ends
//	@Description	it at the API server too. previous=true reads the last terminated
//	@Description	instance, which is where the reason for a CrashLoopBackOff lives.
//	@Tags			kubernetes
//	@Produce		plain
//	@Security		BearerAuth
//	@Param			namespace	path		string	true	"Namespace"
//	@Param			pod			path		string	true	"Pod name"
//	@Param			container	query		string	false	"Container; required when the pod has more than one"
//	@Param			follow		query		boolean	false	"Keep the stream open"
//	@Param			tail		query		integer	false	"Lines of history first (default 200, max 5000)"
//	@Param			previous	query		boolean	false	"Read the last terminated instance"
//	@Success		200			{string}	string	"the log"
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		401			{object}	http.ErrorBody
//	@Failure		403			{object}	http.ErrorBody
//	@Failure		404			{object}	http.ErrorBody
//	@Router			/api/kubernetes/namespaces/{namespace}/pods/{pod}/logs [get]
func (h *Handler) podLogs(w http.ResponseWriter, r *http.Request) {
	if !httpx.ClearWriteDeadline(w, "pod log stream") {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), httpx.MaxStreamDuration)
	defer cancel()

	stream, err := h.service.PodLogs(ctx, r.PathValue("namespace"), r.PathValue("pod"), LogOptions{
		Container: r.URL.Query().Get("container"),
		Follow:    r.URL.Query().Has("follow") && r.URL.Query().Get("follow") != "false",
		Tail:      tailLines(r.URL.Query().Get("tail")),
		Previous:  r.URL.Query().Has("previous") && r.URL.Query().Get("previous") != "false",
	})
	if err != nil {
		// Nothing has been written yet, so this can still be a status code. It is
		// the last point at which that is true.
		respondError(w, err)
		return
	}
	defer func() { _ = stream.Close() }()

	httpx.StreamText(w, r.WithContext(ctx), stream, "pod log stream")
}

// tailLines reads the tail parameter, leaving the service to apply the default.
//
// A malformed value becomes zero rather than an error: the parameter is an
// optimization, and refusing the whole request over it would be a worse answer
// than sending the default amount of history.
func tailLines(raw string) int64 {
	lines, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return lines
}

// respondError maps this slice's errors to status codes.
func respondError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidName):
		httpx.Error(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrUnsupportedKind):
		httpx.Error(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrConflict):
		// Somebody else changed the workload between this request reading it and
		// writing it back. Saying so is more useful than retrying silently, which
		// would let the second operator overwrite the first without either knowing.
		httpx.Error(w, http.StatusConflict, "conflict",
			"the workload changed while this request was in flight — try again")
	case errors.Is(err, ErrProtected):
		// 403 rather than 409: this is a statement about the object, not a
		// temporary condition, so there is nothing to retry. The reason travels
		// with it because the panel shows it next to the namespace.
		httpx.Error(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrAlreadyExists):
		httpx.Error(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, ErrNotEmpty):
		httpx.Error(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, ErrNotManaged):
		// 403, and it names the namespace: the fix is to manage it, which is an
		// operator decision rather than something to retry.
		httpx.Error(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrNotConfigured):
		// 501, the same answer the Helm routes give when nothing is enrolled: the
		// capability is built and this lab has not switched it on, which is
		// neither the caller's mistake nor a failure.
		httpx.Error(w, http.StatusNotImplemented, "not_configured",
			"this panel is not configured to deploy with Helm")
	case errors.Is(err, ErrForbidden):
		// Almost always the panel's own ClusterRole binding rather than anything
		// the caller did, so it says so rather than reading as a 500.
		httpx.Error(w, http.StatusForbidden, "forbidden",
			"the panel's service account is not permitted to read this")
	default:
		slog.Error("kubernetes request failed", slog.Any("error", err))
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "the request could not be completed")
	}
}

// putSecret writes an Opaque Secret into a namespace.
//
//	@Summary		Write a Secret
//	@Description	Creates an Opaque Secret so a credential the panel just issued can
//	@Description	reach the chart that will use it. Refused for a protected namespace,
//	@Description	and refused with 409 when a Secret of that name is already there
//	@Description	unless overwrite is set — replacing one is how a running release
//	@Description	loses the credential it started with. The values are never readable
//	@Description	back through this API: the response carries the keys and nothing else.
//	@Tags			kubernetes
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	path		string			true	"Namespace name"
//	@Param			name		path		string			true	"Secret name"
//	@Param			request		body		kube.SecretSpec	true	"The Secret contents"
//	@Success		201			{object}	kube.SecretRef
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		401			{object}	http.ErrorBody
//	@Failure		403			{object}	http.ErrorBody
//	@Failure		404			{object}	http.ErrorBody
//	@Failure		409			{object}	http.ErrorBody
//	@Router			/api/kubernetes/namespaces/{namespace}/secrets/{name} [put]
func (h *Handler) putSecret(w http.ResponseWriter, r *http.Request) {
	var spec SecretSpec
	if !httpx.DecodeJSON(w, r, &spec) {
		return
	}
	// The name is the path segment, not a body field. One place for it means a
	// request cannot name two different Secrets and leave the reader guessing
	// which one it wrote.
	spec.Name = r.PathValue("name")

	ref, err := h.service.PutSecret(r.Context(), r.PathValue("namespace"), spec)
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, ref)
}

// enrolNamespace creates or repairs the role bindings a Helm target needs.
//
//	@Summary		Enrol a namespace as a Helm target
//	@Description	Creates the role bindings that let the panel read and deploy releases
//	@Description	in this namespace, and replaces any that are there and wrong — a
//	@Description	binding left by an older chart points at a role that no longer exists,
//	@Description	and roleRef is immutable, so nothing else will ever fix it. Idempotent:
//	@Description	setting up and repairing are the same request. Refused for a protected
//	@Description	namespace.
//	@Tags			kubernetes
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	path		string	true	"Namespace name"
//	@Success		200			{object}	kube.Namespace
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		401			{object}	http.ErrorBody
//	@Failure		403			{object}	http.ErrorBody
//	@Failure		404			{object}	http.ErrorBody
//	@Failure		501			{object}	http.ErrorBody
//	@Router			/api/kubernetes/namespaces/{namespace}/helm-enrolment [post]
func (h *Handler) enrolNamespace(w http.ResponseWriter, r *http.Request) {
	namespace, err := h.service.EnrolNamespace(r.Context(), r.PathValue("namespace"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, namespace)
}

// revokeNamespace removes the panel's role bindings from a namespace.
//
//	@Summary		Revoke a namespace's Helm enrolment
//	@Description	Removes the role bindings, after which the panel can neither deploy
//	@Description	into the namespace nor read its releases. The namespace's labels are
//	@Description	left alone: this owns role bindings, and nothing works without them.
//	@Tags			kubernetes
//	@Produce		json
//	@Security		BearerAuth
//	@Param			namespace	path	string	true	"Namespace name"
//	@Success		204
//	@Failure		400	{object}	http.ErrorBody
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Failure		501	{object}	http.ErrorBody
//	@Router			/api/kubernetes/namespaces/{namespace}/helm-enrolment [delete]
func (h *Handler) revokeNamespace(w http.ResponseWriter, r *http.Request) {
	if err := h.service.RevokeNamespace(r.Context(), r.PathValue("namespace")); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
