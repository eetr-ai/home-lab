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
// A key's name travels as the `key` query parameter, percent-encoded like any
// query value — every byte of it, for a key that is not text. Not a path
// segment: URL handling collapses `.` and `..` segments before any router sees
// them, so a key named `..` could be listed and never opened. A query value has
// no such rule, and it is the form a caller with nothing but curl gets right.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/redis/overview", h.overview)
	mux.HandleFunc("GET /api/redis/keys", h.scanKeys)
	mux.HandleFunc("GET /api/redis/key", h.readKey)
	mux.HandleFunc("DELETE /api/redis/key", h.deleteKey)
	mux.HandleFunc("GET /api/redis/users", h.listUsers)
	mux.HandleFunc("POST /api/redis/users", h.createUser)
	mux.HandleFunc("PUT /api/redis/users/{user}", h.updateUser)
	mux.HandleFunc("DELETE /api/redis/users/{user}", h.deleteUser)
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
//	@Param			key	query		string	true	"Key name, percent-encoded as any query value"
//	@Success		200	{object}	redis.KeyDetail
//	@Failure		400	{object}	http.ErrorBody
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Router			/api/redis/key [get]
func (h *Handler) readKey(w http.ResponseWriter, r *http.Request) {
	detail, err := h.service.ReadKey(r.Context(), r.URL.Query().Get("key"))
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
//	@Param			key	query	string	true	"Key name, percent-encoded as any query value"
//	@Success		204
//	@Failure		400	{object}	http.ErrorBody
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Router			/api/redis/key [delete]
func (h *Handler) deleteKey(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteKey(r.Context(), r.URL.Query().Get("key")); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listUsers returns every ACL user.
//
//	@Summary		List Redis users
//	@Description	Every ACL user, the default one included, with the key prefix and preset of
//	@Description	those this panel can edit. persistent is false on a Redis with no aclfile,
//	@Description	where users would not survive a restart and creating one is refused.
//	@Tags			redis
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	redis.UserList
//	@Failure		401	{object}	http.ErrorBody
//	@Router			/api/redis/users [get]
func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.service.ListUsers(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, users)
}

// createUser creates an app's user.
//
//	@Summary		Create a Redis user
//	@Description	A user confined to one key prefix (keys and pub/sub channels), with a
//	@Description	readwrite or readonly preset. The password reaches Redis only as its
//	@Description	SHA-256. Saved to the aclfile; if the save fails the user is removed again.
//	@Tags			redis
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		redis.CreateUserRequest	true	"The user to create"
//	@Success		201		{object}	redis.User
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		401		{object}	http.ErrorBody
//	@Failure		403		{object}	http.ErrorBody
//	@Failure		409		{object}	http.ErrorBody
//	@Router			/api/redis/users [post]
func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var request CreateUserRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}
	user, err := h.service.CreateUser(r.Context(), request)
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, user)
}

// updateUser replaces a user's prefix and preset, and optionally its password.
//
//	@Summary		Update a Redis user
//	@Description	The whole state, not a set of changes. An empty password keeps the current
//	@Description	one, and a disabled user stays disabled. Takes effect at once, including on
//	@Description	connections already authenticated as the user. Refused for the default user
//	@Description	and for any user whose rules or password state the panel did not write.
//	@Tags			redis
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			user	path		string					true	"User name"
//	@Param			request	body		redis.UpdateUserRequest	true	"The desired state"
//	@Success		200		{object}	redis.User
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		401		{object}	http.ErrorBody
//	@Failure		403		{object}	http.ErrorBody
//	@Failure		404		{object}	http.ErrorBody
//	@Failure		409		{object}	http.ErrorBody
//	@Router			/api/redis/users/{user} [put]
func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	var request UpdateUserRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}
	user, err := h.service.UpdateUser(r.Context(), r.PathValue("user"), request)
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, user)
}

// deleteUser removes a user.
//
//	@Summary		Delete a Redis user
//	@Description	Redis closes every connection authenticated as the user immediately, so an
//	@Description	app still using it loses its connection now. Refused for the default user.
//	@Tags			redis
//	@Produce		json
//	@Security		BearerAuth
//	@Param			user	path	string	true	"User name"
//	@Success		204
//	@Failure		400	{object}	http.ErrorBody
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Failure		409	{object}	http.ErrorBody
//	@Router			/api/redis/users/{user} [delete]
func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteUser(r.Context(), r.PathValue("user")); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// notSavedMessage says what state a failed ACL save left the server in.
func notSavedMessage(err error) string {
	if errors.Is(err, errNotUndone) {
		return "Redis would not write the change to its aclfile, and the panel could not fully put things back: " +
			"what Redis does now may not match what it will do after a restart. Check the redis-acl volume and " +
			"this user, then make the change again."
	}
	return "Redis would not write the change to its aclfile, so it was undone. Check the redis-acl volume and try again."
}

// respondError maps this slice's errors to status codes.
//
// Only the slice's own errors reach the caller as prose. Anything else is a
// driver or connection failure whose message can name hosts and internal state,
// so it is logged and answered generically.
func respondError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidRequest), errors.Is(err, ErrWeakPassword):
		httpx.Error(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrAlreadyExists):
		httpx.Error(w, http.StatusConflict, "already_exists", err.Error())
	case errors.Is(err, ErrProtected):
		httpx.Error(w, http.StatusForbidden, "protected", err.Error())
	case errors.Is(err, ErrNotSaved):
		// Before ErrBusy: a change that lost its lock after it was made wraps both,
		// and what the operator needs to hear is that it was not saved.
		//
		// Logged in full, because the wrapped cause is the server's own and is what
		// someone will need. The caller gets our sentence, which says whether it
		// was undone, without the driver's text.
		slog.Error("redis ACL change not saved", slog.Any("error", err))
		httpx.Error(w, http.StatusInternalServerError, "not_saved", notSavedMessage(err))
	case errors.Is(err, ErrBusy):
		httpx.Error(w, http.StatusConflict, "busy", err.Error())
	case errors.Is(err, ErrNotPersistent):
		// 409: the request is fine and the server is in a state that cannot take
		// it yet. The message names the fix.
		httpx.Error(w, http.StatusConflict, "not_persistent", err.Error())
	default:
		slog.Error("redis request failed", slog.Any("error", err))
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "the request could not be completed")
	}
}
