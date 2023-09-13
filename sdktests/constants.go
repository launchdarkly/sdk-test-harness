package sdktests

const (
	// Application tags sent to the cloud are not allowed to have spaces.  However, the SDKs
	// are sanitizing input, so spaces are allowed in the input.  That is why this set
	// of characters includes a space.
	allAllowedTagChars = " ._-abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	tagNameAppID       = "application-id"
	tagNameAppVersion  = "application-version"
)
