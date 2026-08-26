package kube

import (
	"errors"
	"log/slog"
	"net/http"

	httpx "github.com/eetr-ai/home-lab/admin/api/internal/http"
)

// Handler exposes the Kubernetes slice over HTTP. Read-only throughout.
type Handler struct {
	service *Service
}

// NewHandler builds the handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register adds the Kubernetes routes to a mux that already requires a verified
// caller. Every route is a GET, which is the whole surface by design.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/kubernetes/namespaces", h.listNamespaces)
	mux.HandleFunc("GET /api/kubernetes/namespaces/{namespace}/workloads", h.listWorkloads)
	mux.HandleFunc("GET /api/kubernetes/namespaces/{namespace}/pods", h.listPods)
	mux.HandleFunc("GET /api/kubernetes/namespaces/{namespace}/events", h.listEvents)
	mux.HandleFunc("GET /api/kubernetes/nodes", h.listNodes)
	mux.HandleFunc("GET /api/kubernetes/storage", h.readStorage)
	mux.HandleFunc("GET /api/kubernetes/summary", h.readSummary)
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

// respondError maps this slice's errors to status codes.
func respondError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidName):
		httpx.Error(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", err.Error())
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
