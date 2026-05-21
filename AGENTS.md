# SDK Test Harness -- AI Agent Guide

## Spec traceability

Every contract test that exercises a formal SDK specification requirement **must** call
`t.Specification(specID, number, summary)` to record the link at runtime.

### Rules

1. **Placement.** `t.Specification()` calls go first in the test function body, before
   `t.LongRunning()`, `t.RequireCapability()`, or any other `t.*` method.
2. **Multiple specs.** A single test may call `t.Specification()` more than once if it
   exercises multiple requirements.
3. **YAML eval suites.** Use the `specifications` field at suite level instead of Go calls.
   The parameterized test runner propagates these to `t.Specification()` automatically.
4. **Spec source.** Spec IDs (e.g. `HOOK`, `DATASYSTEM`, `FDV2PL`) and requirement numbers
   come from the `sdk-specs` repo's `main` branch.
5. **Line length.** Keep the summary string short enough that the full `t.Specification()`
   call stays under 120 characters. Break onto a second line if needed.
6. **Helper functions.** Functions that don't accept `*ldtest.T` cannot call
   `t.Specification()`. Use a `// SPEC_ID X.Y.Z: summary` comment as a fallback.

### Why

Spec references are surfaced as `<property>` elements in JUnit XML output, making
specification coverage visible in CI dashboards and test reports. This is the Go equivalent
of OpenFeature's `@Specification` annotation (Java) / `[Specification]` attribute (.NET).

### Examples

Go test:

```go
func (c CommonStreamingTests) RecoverableFallback(t *ldtest.T) {
    t.Specification("DATASYSTEM", "1.2.6", "recoverable error triggers fallback")
    t.Specification("CSFDV2", "8.2.1", "recovery timeout default is 300 seconds")
    t.LongRunning()
    // ... test body ...
}
```

YAML eval suite:

```yaml
name: attribute references
specifications:
  - spec: "ATREF"
    number: "1.2.1"
    summary: "plain references treated as literal attribute name"
  - spec: "ATREF"
    number: "1.2.2"
    summary: "~1 escape sequence interpreted as /"
```

## Linting

Run `golangci-lint` after modifying Go files. The repo enforces a 120-character line limit
(`lll`) among other checks. See `.golangci.yml` for the full configuration.
