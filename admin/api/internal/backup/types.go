package backup

// The labels every object this slice creates carries.
//
// managed-by is admin-api, never Helm, for the reason buildJob in the Helm slice
// spells out: Helm adopts what looks like its own, and an upgrade of the panel's
// chart must not delete a schedule somebody made in the panel.
const (
	labelName      = "app.kubernetes.io/name"
	labelManagedBy = "app.kubernetes.io/managed-by"
	managedBy      = "admin-api"

	// tokenComponent labels the Secret holding the Google grant.
	tokenComponent = "admin-backup-token"
)

// GoogleStatus is what the panel needs to draw the connection card and open the
// Picker.
type GoogleStatus struct {
	// Configured says whether this lab has an OAuth client at all. Without one,
	// nothing else here means anything and the page explains how to add it.
	Configured bool `json:"configured"`
	// Connected says whether an account is behind the client.
	Connected bool `json:"connected"`
	// Account is the connected account's email address, when Google shared it.
	Account string `json:"account,omitempty"`
	// ClientID is public by design — it is in the URL the browser is sent to.
	ClientID string `json:"clientId,omitempty"`
	// Scopes is what the consent screen should ask for. The panel builds the
	// authorization URL with exactly these, so the two cannot drift.
	Scopes []string `json:"scopes,omitempty"`
	// PickerAPIKey and PickerAppID configure Google's Picker in the browser. Both
	// are browser credentials, restricted by referrer on Google's side, and meant
	// to be seen.
	PickerAPIKey string `json:"pickerApiKey,omitempty"`
	PickerAppID  string `json:"pickerAppId,omitempty"`
}

// ConnectRequest finishes the OAuth flow the panel started.
type ConnectRequest struct {
	// Code is the authorization code Google handed the panel's callback.
	Code string `json:"code"`
	// CodeVerifier is the PKCE secret the panel kept while the operator was at
	// Google. Proof that whoever finishes the flow is whoever started it.
	CodeVerifier string `json:"codeVerifier"`
	// RedirectURI is the callback the browser was sent back to, which Google
	// checks against the one the flow began with.
	RedirectURI string `json:"redirectUri"`
}
