module github.com/launchdarkly/sdk-test-harness/v2

go 1.17

require (
	github.com/fatih/color v1.13.0
	github.com/gorilla/mux v1.8.0
	github.com/launchdarkly/eventsource v1.6.2
	github.com/launchdarkly/go-test-helpers/v2 v2.3.1
	github.com/stretchr/testify v1.7.0
	gopkg.in/launchdarkly/go-jsonstream.v1 v1.0.1
	gopkg.in/launchdarkly/go-sdk-common.v3 v3.0.0
	gopkg.in/launchdarkly/go-server-sdk-evaluation.v2 v2.0.0-00010101000000-000000000000
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/uuid v1.1.2 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	github.com/launchdarkly/go-semver v1.0.2 // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/mattn/go-colorable v0.1.9 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/sys v0.0.0-20210809222454-d867a43fc93e // indirect
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
)

replace gopkg.in/launchdarkly/go-sdk-common.v3 => github.com/launchdarkly/go-sdk-common-private/v3 v3.0.0-alpha.3

replace gopkg.in/launchdarkly/go-server-sdk-evaluation.v2 => github.com/launchdarkly/go-server-sdk-evaluation-private/v2 v2.0.0-alpha.1

replace gopkg.in/launchdarkly/go-sdk-events.v2 => github.com/launchdarkly/go-sdk-events-private/v2 v2.0.0-alpha.1
