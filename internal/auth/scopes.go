package auth

// OIDC scopes recognised by bass.
const (
	// ScopeAdmin grants admin operations: register apps, list devices, revoke.
	ScopeAdmin = "bass.admin"
	// ScopeSync allows an end user to pair their device with a registered app.
	ScopeSync = "bass.sync"
)
