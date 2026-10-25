package backup

import (
	"errors"
	"log/slog"
	"net/http"

	httpx "github.com/eetr-ai/home-lab/admin/api/internal/http"
)

// Handler exposes the backups slice over HTTP.
type Handler struct {
	service *Service
}

// NewHandler builds the handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register adds the backup routes to a mux that already requires a verified
// caller.
//
// Literal paths in each HandleFunc, because the OpenAPI coverage test reads them
// out of this source.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/backups/google", h.googleStatus)
	mux.HandleFunc("DELETE /api/backups/google", h.disconnectGoogle)
	mux.HandleFunc("POST /api/backups/google/connect", h.connectGoogle)
	mux.HandleFunc("POST /api/backups/google/access-token", h.googleAccessToken)
}

// googleStatus reports whether Google Drive is set up and connected.
//
//	@Summary		Read the Google Drive connection
//	@Description	Whether this lab has an OAuth client, whether an account is connected
//	@Description	to it, and what the panel needs to open Google's folder Picker. Never
//	@Description	carries the grant itself.
//	@Tags			backups
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	backup.GoogleStatus
//	@Failure		401	{object}	http.ErrorBody
//	@Router			/api/backups/google [get]
func (h *Handler) googleStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.service.GoogleStatus(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, status)
}

// connectGoogle finishes the OAuth flow the panel started.
//
//	@Summary		Connect a Google account
//	@Description	Exchanges the authorization code for a grant and keeps it in a Secret
//	@Description	the backup jobs read. Replaces, and revokes, any account connected
//	@Description	before.
//	@Tags			backups
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		backup.ConnectRequest	true	"The code Google returned"
//	@Success		200		{object}	backup.GoogleStatus
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		401		{object}	http.ErrorBody
//	@Failure		501		{object}	http.ErrorBody
//	@Failure		502		{object}	http.ErrorBody
//	@Router			/api/backups/google/connect [post]
func (h *Handler) connectGoogle(w http.ResponseWriter, r *http.Request) {
	var request ConnectRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}
	status, err := h.service.Connect(r.Context(), request)
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, status)
}

// disconnectGoogle revokes and forgets the connected account.
//
//	@Summary		Disconnect Google Drive
//	@Description	Revokes the grant at Google and deletes it here. Scheduled backups
//	@Description	keep their configuration and fail until an account is connected again.
//	@Tags			backups
//	@Security		BearerAuth
//	@Success		204
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		501	{object}	http.ErrorBody
//	@Router			/api/backups/google [delete]
func (h *Handler) disconnectGoogle(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Disconnect(r.Context()); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// googleAccessToken mints a short-lived token for the folder Picker.
//
//	@Summary		Mint a Picker access token
//	@Description	A short-lived drive.file token for Google's Picker, which runs in the
//	@Description	operator's browser and talks to Google directly. Expires within the hour.
//	@Tags			backups
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	backup.AccessToken
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		409	{object}	http.ErrorBody
//	@Failure		501	{object}	http.ErrorBody
//	@Failure		502	{object}	http.ErrorBody
//	@Router			/api/backups/google/access-token [post]
func (h *Handler) googleAccessToken(w http.ResponseWriter, r *http.Request) {
	token, err := h.service.AccessToken(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, token)
}

// respondError maps this slice's errors to a status. It is the only place that
// knows an error is eventually a number.
func respondError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		httpx.Error(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrGoogleRefused):
		// 400, not 502. Google answered, and the answer is something the operator
		// fixes — connect again, or fix the client secret — rather than waits out.
		httpx.Error(w, http.StatusBadRequest, "google_refused", err.Error())
	case errors.Is(err, ErrNotConnected):
		// 409: the request is fine and the lab is not in the state it needs.
		httpx.Error(w, http.StatusConflict, "not_connected",
			"connect a Google account on the backups page first")
	case errors.Is(err, ErrAlreadyExists):
		httpx.Error(w, http.StatusConflict, "already_exists", err.Error())
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrNotConfigured):
		// 501, the same as every other capability this panel built and this lab
		// has not switched on.
		httpx.Error(w, http.StatusNotImplemented, "not_configured",
			"this lab has no Google OAuth client; see docs/backups.md")
	case errors.Is(err, ErrGoogleUnreachable):
		httpx.Error(w, http.StatusBadGateway, "google_unreachable", "Google did not answer; try again")
	case errors.Is(err, ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "forbidden",
			"the panel's service account is not permitted to do this")
	default:
		slog.Error("backup request failed", slog.Any("error", err))
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "the request could not be completed")
	}
}
