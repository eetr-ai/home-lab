package secretgen

import (
	"errors"
	"net/http"
	"strconv"

	httpx "github.com/eetr-ai/home-lab/admin/api/internal/http"
)

// Handler exposes the generator over HTTP.
type Handler struct{}

// NewHandler builds the handler.
func NewHandler() *Handler {
	return &Handler{}
}

// Register adds the generator route to a mux that already requires a verified
// caller.
//
// One route, and it holds no state and reads nothing. It is a GET because it
// changes nothing — but note that it is not idempotent, which is the one way it
// is unlike every other GET in this API: asking twice gives two different
// answers, and a caller that retries has thrown the first one away.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/secret-values", h.generate)
}

// Generated is one minted value.
type Generated struct {
	Shape  string `json:"shape"`
	Value  string `json:"value"`
	Length int    `json:"length"`
}

// generate mints a credential.
//
//	@Summary		Generate a credential
//	@Description	Mints a password or a token, so nobody has to invent one. Shapes:
//	@Description	`password` (letters, digits and shell-safe symbols), `alphanumeric`,
//	@Description	`hex` and `base64` — the last two are 256 bits and ignore `length`,
//	@Description	because that is the requirement rather than a default. `base64` is
//	@Description	the AUTH_SECRET shape, the same thing `npx auth secret` produces.
//	@Description	Drawn from crypto/rand with rejection sampling, so every character is
//	@Description	equally likely.
//	@Description
//	@Description	NOTE that the value travels in the response, which is the point and is
//	@Description	also the cost: a credential minted here lands in the caller's logs and,
//	@Description	if the caller is the assistant, in a model's context and memory. The
//	@Description	panel's own password fields generate in the browser instead, so a value
//	@Description	an operator keeps never leaves it.
//	@Tags			tools
//	@Produce		json
//	@Security		BearerAuth
//	@Param			shape	query		string	false	"password, alphanumeric, hex or base64 (default password)"
//	@Param			length	query		int		false	"12-128, for the sized shapes only (default 24)"
//	@Success		200		{object}	secretgen.Generated
//	@Failure		400		{object}	http.ErrorBody
//	@Failure		401		{object}	http.ErrorBody
//	@Router			/api/secret-values [get]
func (h *Handler) generate(w http.ResponseWriter, r *http.Request) {
	shape := Shape(r.URL.Query().Get("shape"))
	if shape == "" {
		shape = ShapePassword
	}

	length := DefaultLength
	if raw := r.URL.Query().Get("length"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid_request", "length must be a whole number")
			return
		}
		length = parsed
	}

	value, err := Generate(shape, length)
	if err != nil {
		if errors.Is(err, ErrInvalidRequest) {
			httpx.Error(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		// The only other failure is the randomness source, and there is nothing
		// useful to say about it beyond that it happened. Returning a value anyway
		// is the one thing this must never do.
		httpx.Error(w, http.StatusInternalServerError, "internal_error",
			"a value could not be generated")
		return
	}

	httpx.JSON(w, http.StatusOK, Generated{Shape: string(shape), Value: value, Length: len(value)})
}
