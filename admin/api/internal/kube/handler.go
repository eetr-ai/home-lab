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
