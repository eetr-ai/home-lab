package mongo

import (
	"errors"
	"log/slog"
	"net/http"

	httpx "github.com/eetr-ai/home-lab/admin/api/internal/http"
)

// Handler exposes the MongoDB slice over HTTP. It owns transport only.
type Handler struct {
	service *Service
}

// NewHandler builds the handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register adds the MongoDB routes to a mux that already requires a verified
// caller.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/mongo/databases", h.listDatabases)
	mux.HandleFunc("POST /api/mongo/databases", h.createDatabase)
	mux.HandleFunc("DELETE /api/mongo/databases/{database}", h.dropDatabase)
	mux.HandleFunc("GET /api/mongo/databases/{database}/collections", h.listCollections)
	mux.HandleFunc("POST /api/mongo/databases/{database}/collections", h.createCollection)
	mux.HandleFunc("DELETE /api/mongo/databases/{database}/collections/{collection}", h.dropCollection)
	mux.HandleFunc("GET /api/mongo/databases/{database}/users", h.listUsers)
	mux.HandleFunc("POST /api/mongo/databases/{database}/users", h.createUser)
	mux.HandleFunc("DELETE /api/mongo/databases/{database}/users/{user}", h.dropUser)
}

// listDatabases returns every database on the server.
//
//	@Summary		List databases
//	@Tags			mongo
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{array}		mongo.Database
//	@Failure		401	{object}	http.ErrorBody
//	@Router			/api/mongo/databases [get]
func (h *Handler) listDatabases(w http.ResponseWriter, r *http.Request) {
	databases, err := h.service.ListDatabases(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, databases)
}

// createDatabase creates a database by creating its first collection.
//
//	@Summary		Create a database
//	@Description	MongoDB has no standalone create-database command: a database begins to exist
//	@Description	when something is put in it, so the first collection is part of the request.
//	@Tags			mongo
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		mongo.CreateDatabaseRequest	true	"The database and its first collection"
//	@Success		201		{object}	mongo.Database
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		403		{object}	http.ErrorBody
//	@Failure		409		{object}	http.ErrorBody
//	@Router			/api/mongo/databases [post]
func (h *Handler) createDatabase(w http.ResponseWriter, r *http.Request) {
	var request CreateDatabaseRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}
	database, err := h.service.CreateDatabase(r.Context(), request)
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, database)
}

// dropDatabase removes a database and everything in it.
//
//	@Summary		Drop a database
//	@Description	Removes the database and all of its data. MongoDB's own databases are refused.
//	@Tags			mongo
//	@Produce		json
//	@Security		BearerAuth
//	@Param			database	path	string	true	"Database name"
//	@Success		204
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		400	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Router			/api/mongo/databases/{database} [delete]
func (h *Handler) dropDatabase(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DropDatabase(r.Context(), r.PathValue("database")); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listCollections returns the collections in one database.
//
//	@Summary		List collections
//	@Tags			mongo
//	@Produce		json
//	@Security		BearerAuth
//	@Param			database	path		string	true	"Database name"
//	@Success		200			{array}		mongo.Collection
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		400			{object}	http.ErrorBody
//	@Router			/api/mongo/databases/{database}/collections [get]
func (h *Handler) listCollections(w http.ResponseWriter, r *http.Request) {
	collections, err := h.service.ListCollections(r.Context(), r.PathValue("database"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, collections)
}

// createCollection creates a collection in an existing database.
//
//	@Summary		Create a collection
//	@Tags			mongo
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			database	path		string							true	"Database name"
//	@Param			request		body		mongo.CreateCollectionRequest	true	"The collection to create"
//	@Success		201			{object}	mongo.Collection
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		409			{object}	http.ErrorBody
//	@Router			/api/mongo/databases/{database}/collections [post]
func (h *Handler) createCollection(w http.ResponseWriter, r *http.Request) {
	var request CreateCollectionRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}
	collection, err := h.service.CreateCollection(r.Context(), r.PathValue("database"), request)
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, collection)
}

// dropCollection removes a collection and its documents.
//
//	@Summary		Drop a collection
//	@Tags			mongo
//	@Produce		json
//	@Security		BearerAuth
//	@Param			database	path	string	true	"Database name"
//	@Param			collection	path	string	true	"Collection name"
//	@Success		204
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		400	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Router			/api/mongo/databases/{database}/collections/{collection} [delete]
func (h *Handler) dropCollection(w http.ResponseWriter, r *http.Request) {
	err := h.service.DropCollection(r.Context(), r.PathValue("database"), r.PathValue("collection"))
	if err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listUsers returns the users defined in one database.
//
//	@Summary		List users
//	@Description	MongoDB scopes a user to the database it was created in, which is where it authenticates.
//	@Tags			mongo
//	@Produce		json
//	@Security		BearerAuth
//	@Param			database	path		string	true	"Database the users are defined in"
//	@Success		200			{array}		mongo.User
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		400			{object}	http.ErrorBody
//	@Router			/api/mongo/databases/{database}/users [get]
func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.service.ListUsers(r.Context(), r.PathValue("database"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, users)
}

// createUser creates a user in one database.
//
//	@Summary		Create a user
//	@Description	Roles that administer the whole server, such as root or readWriteAnyDatabase, cannot be granted here.
//	@Tags			mongo
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			database	path		string					true	"Database to create the user in"
//	@Param			request		body		mongo.CreateUserRequest	true	"The user to create"
//	@Success		201			{object}	mongo.User
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		409			{object}	http.ErrorBody
//	@Router			/api/mongo/databases/{database}/users [post]
func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var request CreateUserRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}
	user, err := h.service.CreateUser(r.Context(), r.PathValue("database"), request)
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, user)
}

// dropUser removes a user.
//
//	@Summary		Drop a user
//	@Description	The account this panel connects as is refused.
//	@Tags			mongo
//	@Produce		json
//	@Security		BearerAuth
//	@Param			database	path	string	true	"Database the user is defined in"
//	@Param			user		path	string	true	"User name"
//	@Success		204
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		400	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Router			/api/mongo/databases/{database}/users/{user} [delete]
func (h *Handler) dropUser(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DropUser(r.Context(), r.PathValue("database"), r.PathValue("user")); err != nil {
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
	case errors.Is(err, ErrInvalidName), errors.Is(err, ErrWeakPassword), errors.Is(err, ErrInvalidRole):
		httpx.Error(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrAlreadyExists):
		httpx.Error(w, http.StatusConflict, "already_exists", err.Error())
	case errors.Is(err, ErrProtected):
		httpx.Error(w, http.StatusForbidden, "protected", err.Error())
	default:
		slog.Error("mongo request failed", slog.Any("error", err))
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "the request could not be completed")
	}
}
