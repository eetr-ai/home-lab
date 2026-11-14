package redis

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	httpx "github.com/eetr-ai/home-lab/admin/api/internal/http"
)

// Handler exposes the Redis slice over HTTP. It owns transport only.
type Handler struct {
	service *Service
}

// NewHandler builds the handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register adds the Redis routes to a mux that already requires a verified
// caller.
//
// A key's name is a path segment, so the caller percent-encodes it — every byte
// of it, for a key that is not text. The mux decodes it back before the service
// sees it, which is why a key with a slash in it works.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/redis/overview", h.overview)
	mux.HandleFunc("GET /api/redis/keys", h.scanKeys)
	mux.HandleFunc("GET /api/redis/keys/{key}", h.readKey)
	mux.HandleFunc("DELETE /api/redis/keys/{key}", h.deleteKey)
}

// overview reports what the server says about itself.
//
//	@Summary		Read the Redis overview
//	@Description	Version, memory against the ceiling, clients, hit rates and the keyspace,
//	@Description	parsed from INFO. The platform runs Redis with noeviction, so memory at the
//	@Description	ceiling means writes are refused; oomRejections counts the ones already were.
//	@Tags			redis
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	redis.Overview
//	@Failure		401	{object}	http.ErrorBody
//	@Router			/api/redis/overview [get]
func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	overview, err := h.service.Overview(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, overview)
}

// scanKeys returns one page of keys.
//
//	@Summary		Scan keys
//	@Description	One SCAN step over keys matching a glob pattern. A page may be empty while
//	@Description	nextCursor is not: SCAN promises completeness, not page size. Pass nextCursor
//	@Description	back to continue; it is empty when the scan is finished. count is a hint,
//	@Description	capped at 1000.
//	@Tags			redis
//	@Produce		json
//	@Security		BearerAuth
//	@Param			pattern	query		string	false	"Glob pattern; empty matches every key"
//	@Param			cursor	query		string	false	"The nextCursor from the previous page"
//	@Param			count	query		int		false	"How many keys to examine; default 100"
//	@Success		200		{object}	redis.KeyPage
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		401		{object}	http.ErrorBody
//	@Router			/api/redis/keys [get]
func (h *Handler) scanKeys(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	count := 0
	if raw := query.Get("count"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid_request", "count must be a whole number")
			return
		}
		count = n
	}

	page, err := h.service.ScanKeys(r.Context(), query.Get("pattern"), query.Get("cursor"), count)
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

// readKey opens one key.
//
//	@Summary		Read a key
//	@Description	Type, TTL, memory and the first part of the value: the first 100 items of a
//	@Description	collection or the first 64 KiB of a string, with truncated saying whether
//	@Description	there was more. Bytes that are not UTF-8 come back base64, with encoding
//	@Description	saying so.
//	@Tags			redis
//	@Produce		json
//	@Security		BearerAuth
//	@Param			key	path		string	true	"Key name, percent-encoded"
//	@Success		200	{object}	redis.KeyDetail
//	@Failure		400	{object}	http.ErrorBody
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Router			/api/redis/keys/{key} [get]
func (h *Handler) readKey(w http.ResponseWriter, r *http.Request) {
	detail, err := h.service.ReadKey(r.Context(), r.PathValue("key"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, detail)
}

// deleteKey removes one key.
//
//	@Summary		Delete a key
//	@Description	UNLINK, so a large value is freed without stalling the server. There is no
//	@Description	way to delete more than one key at a time here, on purpose.
//	@Tags			redis
//	@Produce		json
//	@Security		BearerAuth
//	@Param			key	path	string	true	"Key name, percent-encoded"
//	@Success		204
//	@Failure		400	{object}	http.ErrorBody
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Router			/api/redis/keys/{key} [delete]
func (h *Handler) deleteKey(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteKey(r.Context(), r.PathValue("key")); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// respondError maps this slice's errors to status codes.
//
// Only the slice's own errors reach the caller as prose. Anything else is a
// driver or connection failure whose message can name hosts and internal state,
// so it is logged and answered generically.
func respondError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidRequest):
		httpx.Error(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", err.Error())
	default:
		slog.Error("redis request failed", slog.Any("error", err))
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "the request could not be completed")
	}
}
