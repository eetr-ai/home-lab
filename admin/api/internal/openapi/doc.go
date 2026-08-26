// Package openapi serves the admin API's own description.
//
// The document is generated from the annotations on the handlers by
// `task admin-api:openapi` and committed as swagger.json beside this file, then
// embedded into the binary. Generation is strictly a build-time step: nothing
// here parses Go source at runtime, and the swaggo toolchain is not a dependency
// of the module — only of the task that regenerates the artifact. CI regenerates
// and fails on a diff, so the committed spec and the annotations cannot drift.
//
// It exists because a description of this API is the difference between a caller
// that has to be told every route and one that can look them up. The assistant
// agent is the first such caller, and the reason the API is a service at all
// rather than logic inside the panel.
//
// The general annotations live here rather than in main.go so that file stays
// about wiring.
//
//	@title			home-lab Admin API
//	@version		0.1.0
//	@description	Manages the home lab's PostgreSQL and MongoDB services, and reads its Kubernetes cluster.
//	@description	Every /api endpoint needs a bearer access token from the configured OpenID Connect
//	@description	provider. There are no API keys: a caller acts as the person who signed in, and an
//	@description	agent carries that person's token.
//	@license.name	MIT
//
//	@securityDefinitions.bearerauth	BearerAuth
//	@description					An OAuth 2.1 access token from the configured provider.
package openapi
