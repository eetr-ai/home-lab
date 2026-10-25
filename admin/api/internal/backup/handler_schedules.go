package backup

import (
	"net/http"

	"github.com/eetr-ai/home-lab/admin/api/internal/auth"
	httpx "github.com/eetr-ai/home-lab/admin/api/internal/http"
)

func (h *Handler) registerSchedules(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/backups/schedules", h.listSchedules)
	mux.HandleFunc("POST /api/backups/schedules", h.createSchedule)
	mux.HandleFunc("GET /api/backups/schedules/{name}", h.readSchedule)
	mux.HandleFunc("PUT /api/backups/schedules/{name}", h.updateSchedule)
	mux.HandleFunc("DELETE /api/backups/schedules/{name}", h.deleteSchedule)
	mux.HandleFunc("POST /api/backups/schedules/{name}/run", h.runSchedule)
	mux.HandleFunc("GET /api/backups/schedules/{name}/runs", h.listRuns)
	mux.HandleFunc("GET /api/backups/schedules/{name}/files", h.listFiles)
}

// listSchedules returns every scheduled backup.
//
//	@Summary		List backup schedules
//	@Description	Each schedule is a CronJob in the panel's namespace, read back as it is.
//	@Tags			backups
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{array}		backup.Schedule
//	@Failure		401	{object}	http.ErrorBody
//	@Router			/api/backups/schedules [get]
func (h *Handler) listSchedules(w http.ResponseWriter, r *http.Request) {
	schedules, err := h.service.ListSchedules(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, schedules)
}

// createSchedule declares a new scheduled backup.
//
//	@Summary		Create a backup schedule
//	@Description	Creates the CronJob. The cron line is checked by the Kubernetes API
//	@Description	server, and a refusal comes back as a 400 that names it.
//	@Tags			backups
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		backup.ScheduleRequest	true	"The schedule"
//	@Success		201		{object}	backup.Schedule
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		401		{object}	http.ErrorBody
//	@Failure		409		{object}	http.ErrorBody
//	@Failure		501		{object}	http.ErrorBody
//	@Router			/api/backups/schedules [post]
func (h *Handler) createSchedule(w http.ResponseWriter, r *http.Request) {
	var request ScheduleRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}
	schedule, err := h.service.CreateSchedule(r.Context(), request, actorOf(r))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, schedule)
}

// readSchedule returns one schedule.
//
//	@Summary		Read a backup schedule
//	@Tags			backups
//	@Produce		json
//	@Security		BearerAuth
//	@Param			name	path		string	true	"Schedule name"
//	@Success		200		{object}	backup.Schedule
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		401		{object}	http.ErrorBody
//	@Failure		404		{object}	http.ErrorBody
//	@Router			/api/backups/schedules/{name} [get]
func (h *Handler) readSchedule(w http.ResponseWriter, r *http.Request) {
	schedule, err := h.service.ReadSchedule(r.Context(), r.PathValue("name"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, schedule)
}

// updateSchedule replaces a schedule.
//
//	@Summary		Replace a backup schedule
//	@Description	The path names the schedule; a name in the body is ignored.
//	@Tags			backups
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			name	path		string					true	"Schedule name"
//	@Param			request	body		backup.ScheduleRequest	true	"The schedule"
//	@Success		200		{object}	backup.Schedule
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		401		{object}	http.ErrorBody
//	@Failure		404		{object}	http.ErrorBody
//	@Failure		409		{object}	http.ErrorBody
//	@Router			/api/backups/schedules/{name} [put]
func (h *Handler) updateSchedule(w http.ResponseWriter, r *http.Request) {
	var request ScheduleRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}
	schedule, err := h.service.UpdateSchedule(r.Context(), r.PathValue("name"), request, actorOf(r))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, schedule)
}

// deleteSchedule removes a schedule and its runs.
//
//	@Summary		Delete a backup schedule
//	@Description	Deletes the CronJob and its runs. The backups already in Drive stay.
//	@Tags			backups
//	@Security		BearerAuth
//	@Param			name	path	string	true	"Schedule name"
//	@Success		204
//	@Failure		400	{object}	http.ErrorBody
//	@Failure		401	{object}	http.ErrorBody
//	@Failure		404	{object}	http.ErrorBody
//	@Router			/api/backups/schedules/{name} [delete]
func (h *Handler) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteSchedule(r.Context(), r.PathValue("name")); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// runSchedule starts a run now.
//
//	@Summary		Run a backup schedule now
//	@Description	Starts a Job from the schedule's template, owned by its CronJob. Runs
//	@Description	on a suspended schedule too: suspending stops the clock, not a person.
//	@Description	Refused with 409 while another run of the schedule is still going.
//	@Tags			backups
//	@Produce		json
//	@Security		BearerAuth
//	@Param			name	path		string	true	"Schedule name"
//	@Success		202		{object}	backup.Run
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		401		{object}	http.ErrorBody
//	@Failure		404		{object}	http.ErrorBody
//	@Failure		409		{object}	http.ErrorBody
//	@Router			/api/backups/schedules/{name}/run [post]
func (h *Handler) runSchedule(w http.ResponseWriter, r *http.Request) {
	run, err := h.service.RunSchedule(r.Context(), r.PathValue("name"), actorOf(r))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, run)
}

// listRuns returns a schedule's recent runs.
//
//	@Summary		List a schedule's runs
//	@Description	Newest first, as many as the CronJob's history keeps.
//	@Tags			backups
//	@Produce		json
//	@Security		BearerAuth
//	@Param			name	path		string	true	"Schedule name"
//	@Success		200		{array}		backup.Run
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		401		{object}	http.ErrorBody
//	@Failure		404		{object}	http.ErrorBody
//	@Router			/api/backups/schedules/{name}/runs [get]
func (h *Handler) listRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.service.ListRuns(r.Context(), r.PathValue("name"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, runs)
}

// listFiles returns the backups a schedule has kept in Drive.
//
//	@Summary		List a schedule's backups
//	@Description	Read from the Drive folder itself, so this is what is actually there.
//	@Tags			backups
//	@Produce		json
//	@Security		BearerAuth
//	@Param			name	path		string	true	"Schedule name"
//	@Success		200		{array}		backup.File
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		401		{object}	http.ErrorBody
//	@Failure		404		{object}	http.ErrorBody
//	@Failure		409		{object}	http.ErrorBody
//	@Failure		502		{object}	http.ErrorBody
//	@Router			/api/backups/schedules/{name}/files [get]
func (h *Handler) listFiles(w http.ResponseWriter, r *http.Request) {
	files, err := h.service.ListFiles(r.Context(), r.PathValue("name"))
	if err != nil {
		respondError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, files)
}

// actorOf is who asked, for the annotation on what they created. An email when the
// token carries one, the client id for a pipeline's token that does not.
func actorOf(r *http.Request) string {
	subject, _ := auth.SubjectFrom(r.Context())
	if subject.Email != "" {
		return subject.Email
	}
	return subject.ClientID
}
