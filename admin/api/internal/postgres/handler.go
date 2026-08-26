package postgres

import (
	"errors"
	"log/slog"
	"net/http"

	httpx "github.com/eetr-ai/home-lab/admin/api/internal/http"
)

// Handler exposes the PostgreSQL slice over HTTP. It owns transport only: it
// decodes, delegates, and maps the slice's errors to status codes.
type Handler struct {
	service *Service
}

// NewHandler builds the handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register adds the PostgreSQL routes to a mux that already requires a verified
// caller.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/postgres/databases", h.listDatabases)
	mux.HandleFunc("POST /api/postgres/databases", h.createDatabase)
	mux.HandleFunc("DELETE /api/postgres/databases/{database}", h.dropDatabase)
	mux.HandleFunc("GET /api/postgres/databases/{database}/extensions", h.listExtensions)
	mux.HandleFunc("POST /api/postgres/databases/{database}/extensions", h.createExtension)
	mux.HandleFunc("GET /api/postgres/roles", h.listRoles)
	mux.HandleFunc("POST /api/postgres/roles", h.createRole)
	mux.HandleFunc("DELETE /api/postgres/roles/{role}", h.dropRole)
	mux.HandleFunc("PUT /api/postgres/roles/{role}", h.updateRole)
	mux.HandleFunc("PUT /api/postgres/databases/{database}", h.updateDatabase)
	mux.HandleFunc("POST /api/postgres/databases/{database}/query", h.query)
}

// updateRole sets a role's flags, connection limit, and optionally its password.
//
//	@Summary		Update a role
//	@Description	The whole desired state, not a set of changes: every attribute is written,
//	@Description	including the negative form, because ALTER ROLE leaves an unmentioned one
//	@Description	alone. An empty password leaves the existing one in place; a supplied one
//	@Description	is converted to a SCRAM verifier before it reaches the server.
//	@Tags			postgres
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			role	path		string						true	"Role name"
//	@Param			request	body		postgres.UpdateRoleRequest	true	"The desired state"
//	@Success		200		{object}	postgres.Role
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		401		{object}	http.ErrorBody
//	@Failure		403		{object}	http.ErrorBody
//	@Failure		404		{object}	http.ErrorBody
//	@Router			/api/postgres/roles/{role} [put]
func (h *Handler) updateRole(w http.ResponseWriter, r *http.Request) {
	var request UpdateRoleRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	role, err := h.service.UpdateRole(r.Context(), r.PathValue("role"), request)
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, role)
}

// updateDatabase reassigns a database to another role.
//
//	@Summary		Update a database
//	@Description	Only the owner. Encoding cannot be changed after creation, and a rename
//	@Description	would break every connection string pointing at it.
//	@Tags			postgres
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			database	path	string							true	"Database name"
//	@Param			request		body	postgres.UpdateDatabaseRequest	true	"The desired owner"
//	@Success		204
//	@Failure		400	{object}	http.ErrorBody
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Router			/api/postgres/databases/{database} [put]
func (h *Handler) updateDatabase(w http.ResponseWriter, r *http.Request) {
	var request UpdateDatabaseRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	if err := h.service.UpdateDatabase(r.Context(), r.PathValue("database"), request); err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusNoContent, nil)
}

// query runs one read-only statement against a database.
//
//	@Summary		Run a read-only query
//	@Description	POST because the statement goes in the body — it is a read, not a change.
//	@Description	Runs over a separate connection authenticated as a non-superuser role, which
//	@Description	is the boundary: dropping privileges within the panel's own superuser
//	@Description	session is reversible by a submitted RESET ROLE. A READ ONLY transaction and
//	@Description	a statement timeout sit behind that. Answers 503 when no such credential is
//	@Description	configured, rather than falling back to the superuser. Results are capped at
//	@Description	200 rows and the statement at 15 seconds.
//	@Tags			postgres
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			database	path		string					true	"Database to run against"
//	@Param			request		body		postgres.QueryRequest	true	"The statement"
//	@Success		200			{object}	postgres.QueryResult
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		401			{object}	http.ErrorBody
//	@Failure		422			{object}	http.ErrorBody
//	@Failure		503			{object}	http.ErrorBody
//	@Router			/api/postgres/databases/{database}/query [post]
func (h *Handler) query(w http.ResponseWriter, r *http.Request) {
	var request QueryRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	result, err := h.service.Query(r.Context(), r.PathValue("database"), request.SQL)
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

// listDatabases returns every database on the server.
//
//	@Summary		List databases
//	@Tags			postgres
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{array}		postgres.Database
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		500	{object}	http.ErrorBody
//	@Router			/api/postgres/databases [get]
func (h *Handler) listDatabases(w http.ResponseWriter, r *http.Request) {
	databases, err := h.service.ListDatabases(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, databases)
}

// createDatabase creates a database. It installs no extension; ask for those
// separately, because an extension belongs to one database rather than the server.
//
//	@Summary		Create a database
//	@Tags			postgres
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		postgres.CreateDatabaseRequest	true	"The database to create"
//	@Success		201		{object}	postgres.Database
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		404		{object}	http.ErrorBody
//	@Failure		409		{object}	http.ErrorBody
//	@Router			/api/postgres/databases [post]
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
//	@Description	Removes the database and all of its data. PostgreSQL's own databases are refused.
//	@Tags			postgres
//	@Produce		json
//	@Security		BearerAuth
//	@Param			database	path	string	true	"Database name"
//	@Success		204
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		400	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Router			/api/postgres/databases/{database} [delete]
func (h *Handler) dropDatabase(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DropDatabase(r.Context(), r.PathValue("database")); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listExtensions returns the extensions installed in one database.
//
//	@Summary		List a database's extensions
//	@Tags			postgres
//	@Produce		json
//	@Security		BearerAuth
//	@Param			database	path		string	true	"Database name"
//	@Success		200			{array}		postgres.Extension
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		404			{object}	http.ErrorBody
//	@Router			/api/postgres/databases/{database}/extensions [get]
func (h *Handler) listExtensions(w http.ResponseWriter, r *http.Request) {
	extensions, err := h.service.ListExtensions(r.Context(), r.PathValue("database"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, extensions)
}

// createExtension installs an extension into one database.
//
//	@Summary		Install an extension
//	@Description	Extensions are per database, so this installs into the named one only.
//	@Description	The server image ships pgvector, whose extension name is "vector".
//	@Tags			postgres
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			database	path		string							true	"Database name"
//	@Param			request		body		postgres.CreateExtensionRequest	true	"The extension to install"
//	@Success		201			{object}	postgres.Extension
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		400			{object}	http.ErrorBody
//	@Failure		404			{object}	http.ErrorBody
//	@Router			/api/postgres/databases/{database}/extensions [post]
func (h *Handler) createExtension(w http.ResponseWriter, r *http.Request) {
	var request CreateExtensionRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}
	extension, err := h.service.CreateExtension(r.Context(), r.PathValue("database"), request)
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, extension)
}

// listRoles returns every role on the server.
//
//	@Summary		List roles
//	@Tags			postgres
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{array}		postgres.Role
//	@Failure		401	{object}	http.ErrorBody
//	@Router			/api/postgres/roles [get]
func (h *Handler) listRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.service.ListRoles(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, roles)
}

// createRole creates a role. Superuser is not offered.
//
//	@Summary		Create a role
//	@Description	A role that can log in needs a password. Superuser cannot be granted through this API.
//	@Tags			postgres
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		postgres.CreateRoleRequest	true	"The role to create"
//	@Success		201		{object}	postgres.Role
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		409		{object}	http.ErrorBody
//	@Router			/api/postgres/roles [post]
func (h *Handler) createRole(w http.ResponseWriter, r *http.Request) {
	var request CreateRoleRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}
	role, err := h.service.CreateRole(r.Context(), request)
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, role)
}

// dropRole removes a role.
//
//	@Summary		Drop a role
//	@Description	PostgreSQL's own roles and the account this panel connects as are refused.
//	@Tags			postgres
//	@Produce		json
//	@Security		BearerAuth
//	@Param			role	path	string	true	"Role name"
//	@Success		204
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		400	{object}	http.ErrorBody
//	@Failure		403	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Router			/api/postgres/roles/{role} [delete]
func (h *Handler) dropRole(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DropRole(r.Context(), r.PathValue("role")); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// respondError maps this slice's errors to status codes.
//
// Only the slice's own errors reach the caller as prose. Anything else is a
// PostgreSQL or connection failure whose message can name hosts, roles, and
// internal SQL, so it is logged and answered generically.
func respondError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidName), errors.Is(err, ErrWeakPassword):
		httpx.Error(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrAlreadyExists):
		httpx.Error(w, http.StatusConflict, "already_exists", err.Error())
	case errors.Is(err, ErrProtected):
		httpx.Error(w, http.StatusForbidden, "protected", err.Error())
	case errors.Is(err, ErrQueryUnavailable):
		httpx.Error(w, http.StatusServiceUnavailable, "not_configured",
			"the query console needs a read-only PostgreSQL credential "+
				"(ADMIN_POSTGRES_QUERY_DSN); see databases/README.md")
	case errors.Is(err, ErrQueryFailed):
		// 422 rather than 400: the request was well-formed and the statement was
		// the thing the server would not accept. The message is PostgreSQL's own,
		// which names the syntax position or the write it refused — and that is the
		// whole value of a query console.
		httpx.Error(w, http.StatusUnprocessableEntity, "query_failed", err.Error())
	default:
		slog.Error("postgres request failed", slog.Any("error", err))
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "the request could not be completed")
	}
}
