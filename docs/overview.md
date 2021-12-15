# Purpose and overview

There are many LaunchDarkly SDKs for many languages and platforms. The design and capabilities of each may vary, but they share a core set of features and it is important to verify that they work consistently across SDKs.

It is impractical to implement full cross-platform coverage only through unit tests in each project: inevitably some test cases will be missed, or new tests will be thought of but only added to some projects, and even if no such mistakes are made, the work of writing these repetitive tests is very tedious.

LaunchDarkly has for a long time performed cross-platform end-to-end integration testing using a test harness approach, where a single test harness application runs many parameterized tests against service APIs that are implemented for each SDK. But, in the past, this approach was difficult to scale since it required a real set of LaunchDarkly services, and could only communicate with the SDKs indirectly through those services: for instance, flag data could only be provided through the streaming service by really creating flags, and event data could only be observed by subscribing to data export.

The SDK test harness in this project is a partial replacement for other testing approaches. Similar to the older end-to-end tests, it expects a small test service to be implemented for each SDK (see [SDK test service specification](./service_spec)) that supports a standardized API with operations like "evaluate a flag". The main differences are that the test harness is fully in control of the configuration of SDK client instances created by the test service, and that the test harness can simulate LaunchDarkly service endpoints. This allows us to do cross-platform _contract tests_ rather than full end-to-end integration tests, although the latter are also possible.

## Contract testing

Contract tests verify that a given component behaves as expected at the boundaries between it and other components: if it receives certain inputs that other components would provide in the real system, then it will produce the kind of outputs and side effects that we would expect in the real system. This does not mean that the entire system (in this case, SDKs plus application code plus the LaunchDarkly service endpoints) is correct, but as long as the contract tests match the specification, we can show that every implementation of the spec (that is, every SDK) is compliant with the spec under a wide range of conditions.

The SDK test harness does this by providing a controlled simulation of LaunchDarkly service endpoints. For instance, if the desired test scenario is "SDK connects to the LaunchDarkly stream, gets such-and-such flag data, evaluates a flag, and sends events", the test harness opens two callback endpoints representing the streaming service and the events service, puts the URIs of those endpoints into the SDK configuration, provides the expected data from the stream, and verifies that it receives the events.

This has several advantages:

* Isolating problems. For instance, if there is a problem with the LaunchDarkly endpoints that the SDK would use to get flag data, but the SDK can correctly evaluate flags and generate events once it has the data, an end-to-end integration test with real services would not be able to show whether evaluation and events are working.
* A level of control that isn't possible with real services. For instance, the test harness can simulate error conditions where a service endpoint is unavailable or sends invalid data, to verify that the SDK handles these errors as expected. Also, in areas where the relevant protocols allow for a range of behavior but the real system currently happens to behave in just one way, the test harness can verify that other allowable behaviors also work (for instance, that the SDKs can correctly parse JSON numbers in the several different formats that JSON allows).

The contract tests provided by this tool are end-to-end tests _of the SDK_, in the sense that the SDK is doing HTTP interactions as it would in real life and that we are only interacting with the SDK through its regular public API. But they are not end-to-end tests _of the whole LaunchDarkly product_.

## Full end-to-end integration testing

The same architecture could also be used for the equivalent of the previously implemented end-to-end integration tests. Design for this is still under construction, but the general approach would be:

* When the test harness sends an SDK configuration to the test service, it does not provide its own callback URIs for service endpoints, but lets the SDK connect to the real services.
* In tests where the test harness would be providing fake flag data, it instead creates real flags.
* In tests where the test harness would be receiving event data directly, it instead subscribes to data export.

That would be similar to the previous approach, with two main differences:

1. Instead of the test services always being deployed and configured ahead of time as they currently are, with an entire instance being required for each configuration under test (such as streaming vs. polling), the test harness could provide SDK configurations on the fly and a single test service instance could handle every configuration for a given SDK.
2. Because so many details of SDK behavior can be verified by contract testing, the much slower full end-to-end approach can be used more selectively for a subset of test cases. For instance, if we can show that the SDK accurately receives flag data for real flags from the real stream, and we can show that the SDK correctly evaluates various flag configurations received from a fake stream, then we do not need to do every one of those evaluation tests with real flags.
