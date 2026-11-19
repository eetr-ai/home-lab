package kube

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	httpx "github.com/eetr-ai/home-lab/admin/api/internal/http"
)

// secretEditBody is SecretEdit with the two maps left undecoded, so a key named
// twice inside one of them can be seen before encoding/json quietly keeps the
// last one.
type secretEditBody struct {
	Replace json.RawMessage `json:"replace,omitempty"`
	Add     json.RawMessage `json:"add,omitempty"`
	Remove  []string        `json:"remove,omitempty"`
}

// decodeSecretEdit reads an edit request, refusing a key repeated inside
// "replace" or "add".
//
// encoding/json resolves {"password": "a", "password": "b"} to "b" without a
// word, so a caller who sent two values for one key would get a 200 for a value
// it may not have meant. validateSecretEdit refuses a key named twice across the
// lists or within "remove"; this is the one place the repetition is still
// visible for the two maps, because after decoding it is gone. Kept to this
// route rather than put in httpx.DecodeJSON, which every other handler shares.
func decodeSecretEdit(w http.ResponseWriter, r *http.Request) (SecretEdit, bool) {
	var body secretEditBody
	if !httpx.DecodeJSON(w, r, &body) {
		return SecretEdit{}, false
	}

	replace, err := decodeUniqueStringMap(body.Replace)
	if err == nil {
		var add map[string]string
		add, err = decodeUniqueStringMap(body.Add)
		if err == nil {
			return SecretEdit{Replace: replace, Add: add, Remove: body.Remove}, true
		}
	}

	var dup duplicateMemberError
	if errors.As(err, &dup) {
		httpx.Error(w, http.StatusBadRequest, "invalid_request", dup.Error())
	} else {
		httpx.Error(w, http.StatusBadRequest, "invalid_request",
			"the request body is not valid JSON for this endpoint")
	}
	return SecretEdit{}, false
}

type duplicateMemberError struct{ key string }

func (e duplicateMemberError) Error() string {
	return fmt.Sprintf("%q is named more than once in the same object", e.key)
}

// decodeUniqueStringMap decodes a JSON object of strings, failing on a repeated
// member. Absent and null both come back as a nil map.
func decodeUniqueStringMap(raw json.RawMessage) (map[string]string, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	open, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("read object: %w", err)
	}
	if delim, ok := open.(json.Delim); !ok || delim != '{' {
		return nil, errors.New("not an object")
	}

	values := map[string]string{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("read key: %w", err)
		}
		key, _ := token.(string) // an object key is always a string token
		if _, seen := values[key]; seen {
			return nil, duplicateMemberError{key: key}
		}
		var value string
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("read value of %q: %w", key, err)
		}
		values[key] = value
	}
	return values, nil
}
