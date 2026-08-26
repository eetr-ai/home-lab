package openapi

import _ "embed"

// The generated description, embedded so the binary is self-describing and needs
// nothing alongside it to answer for its own API.
//
//go:embed swagger.json
var document []byte

// Spec returns the embedded OpenAPI document.
func Spec() []byte {
	return document
}
