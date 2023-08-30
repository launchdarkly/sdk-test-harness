module github.com/launchdarkly/sdk-test-harness/v2

go 1.18

require (
	github.com/fatih/color v1.13.0
	github.com/gorilla/mux v1.8.0
	github.com/launchdarkly/eventsource v1.6.2
	github.com/launchdarkly/go-jsonstream/v3 v3.0.0
	github.com/launchdarkly/go-sdk-common/v3 v3.0.1
	github.com/launchdarkly/go-server-sdk-evaluation/v3 v3.0.0
	github.com/launchdarkly/go-test-helpers/v2 v2.3.2
	github.com/stretchr/testify v1.7.0
	golang.org/x/exp v0.0.0-20220823124025-807a23277127
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/launchdarkly/go-semver v1.0.2 // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/mattn/go-colorable v0.1.9 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.9.0 // indirect
	golang.org/x/sys v0.0.0-20220722155257-8c9f86f7a55f // indirect
)

replace github.com/launchdarkly/go-sdk-common/v3 => github.com/launchdarkly/go-sdk-common-private/v3 v3.0.0-alpha.6.0.20230829225529-e3a87e3952ac

replace github.com/launchdarkly/go-server-sdk-evaluation/v3 => github.com/launchdarkly/go-server-sdk-evaluation-private/v3 v3.0.0-20230829233102-4fc0fa5a3369
