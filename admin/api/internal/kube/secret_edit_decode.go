package kube

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	httpx "github.com/eetr-ai/home-lab/admin/api/internal/http"
)

// decodeSecretEdit reads an edit request, refusing any member named twice —
// "replace", "add" or "remove" at the top level, or one key inside "replace" or
// "add".
//
// encoding/json resolves a repeated member to the last one without a word, so
// {"password": "a", "password": "b"} — or two whole "replace" objects — would
// get a 200 for a value the caller may not have meant. validateSecretEdit
// refuses a key named twice across the lists or within "remove"; this is the one
// place the other repetitions are still visible, because after decoding they are
// gone. Kept to this route rather than put in httpx.DecodeJSON, which every
// other handler shares. Unknown members are refused here too, as DecodeJSON's
// DisallowUnknownFields would have.
func decodeSecretEdit(w http.ResponseWriter, r *http.Request) (SecretEdit, bool) {
	edit, err := parseSecretEdit(r.Body)
	if err == nil {
		return edit, true
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

func parseSecretEdit(body io.Reader) (SecretEdit, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return SecretEdit{}, fmt.Errorf("read body: %w", err)
	}
	members, err := decodeUniqueObject(raw)
	if err != nil {
		return SecretEdit{}, err
	}
	for name := range members {
		if name != "replace" && name != "add" && name != "remove" {
			return SecretEdit{}, fmt.Errorf("unknown member %q", name)
		}
	}

	var edit SecretEdit
	if edit.Replace, err = decodeUniqueStringMap(members["replace"]); err != nil {
		return SecretEdit{}, err
	}
	if edit.Add, err = decodeUniqueStringMap(members["add"]); err != nil {
		return SecretEdit{}, err
	}
	if remove := members["remove"]; len(remove) > 0 {
		if err := json.Unmarshal(remove, &edit.Remove); err != nil {
			return SecretEdit{}, fmt.Errorf("read remove: %w", err)
		}
	}
	return edit, nil
}

type duplicateMemberError struct{ key string }

func (e duplicateMemberError) Error() string {
	return fmt.Sprintf("%q is named more than once in the same object", e.key)
}

// decodeUniqueObject splits a JSON object into its members, failing on a
// repeated one. Each value is left undecoded.
func decodeUniqueObject(raw []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	open, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("read object: %w", err)
	}
	if delim, ok := open.(json.Delim); !ok || delim != '{' {
		return nil, errors.New("not an object")
	}

	members := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("read key: %w", err)
		}
		key, _ := token.(string) // an object key is always a string token
		if _, seen := members[key]; seen {
			return nil, duplicateMemberError{key: key}
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("read value of %q: %w", key, err)
		}
		members[key] = value
	}
	// More() reports false at EOF and before a stray ']' alike, so the loop
	// ending says nothing about whether the object was closed. The closing
	// brace has to be read to know the body was whole.
	closing, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("read end of object: %w", err)
	}
	if delim, ok := closing.(json.Delim); !ok || delim != '}' {
		return nil, errors.New("object not closed")
	}
	return members, nil
}

// decodeUniqueStringMap decodes a JSON object of strings, failing on a repeated
// member. Absent and null both come back as a nil map.
func decodeUniqueStringMap(raw json.RawMessage) (map[string]string, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	members, err := decodeUniqueObject(raw)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string, len(members))
	for key, member := range members {
		var value string
		if err := json.Unmarshal(member, &value); err != nil {
			return nil, fmt.Errorf("read value of %q: %w", key, err)
		}
		values[key] = value
	}
	return values, nil
}
