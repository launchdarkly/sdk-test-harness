package helpers

// TestContext is a minimal interface for types like *testing.T and *ldtest.T representing a
// test that can fail. Functions can use this to avoid specific dependencies on those packages.
type TestContext interface {
	Errorf(msgFormat string, msgArgs ...interface{})
	FailNow()
}
