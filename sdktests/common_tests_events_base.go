package sdktests

import "time"

const defaultEventTimeout = time.Second * 5

// CommonEventTests groups together event-related test methods that are shared between server-side and client-side.
type CommonEventTests struct {
	SDKConfigurers []SDKConfigurer
}
