package helpers

// ConfigOption is an interface for use with the vararg options pattern and ApplyOptions.
type ConfigOption[T any] interface {
	// Configure makes whatever configuration change the option represents.
	Configure(*T) error
}

// ApplyOptions calls any number of ConfigOption implementations against the target value.
// If any returns an error, it immediately stops and returns that error.
func ApplyOptions[T any, U ConfigOption[T]](target *T, options ...U) error {
	// Having a U type parameter, instead of just declaring options as "...ConfigOption[T]",
	// duck-types the interface so the caller can use their own type name if preferred.
	for _, o := range options {
		if err := o.Configure(target); err != nil {
			return err
		}
	}
	return nil
}
