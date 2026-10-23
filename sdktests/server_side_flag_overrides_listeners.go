package sdktests

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"

	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"
)

// The override notification tests start the SDK with this LaunchDarkly data, built with the
// helpers of the listener dependency tests. An arrow points from an item to the item it depends
// on.
//
//	top -> leaf                           prerequisite
//	segment-flag -> segment               flag rule with a segmentMatch clause
//	segment-prereq-flag -> segment-flag   prerequisite on the flag whose rule matches the segment
//	plain                                 no dependencies
//
// Override files then add, change, and remove entries for these keys, or supply override flags
// of their own that depend on the LaunchDarkly items. Adding, changing, or removing an override
// entry must notify the affected flags the same way an equivalent change from LaunchDarkly
// would, including the flags that depend on the entry, and a LaunchDarkly change must notify the
// override flags that depend on the changed item.
const (
	ovlFlagPlain             = "plain"
	ovlFlagOverrideDependent = "override-dependent"
	ovlFlagOverrideSegment   = "override-segment-flag"
	ovlFlagOverrideViaPrereq = "override-segment-prereq-flag"
)

var ovlOverrideValue = ldvalue.String("override-value") //nolint:gochecknoglobals

// overrideListenerFixture is an SDK client started with the LaunchDarkly data above and one
// override file, plus the pusher through which the tests change the LaunchDarkly data.
type overrideListenerFixture struct {
	*streamPusher
	client *SDKClient
	file   *OverrideFile
}

func newOverrideListenerFixture(t *ldtest.T, mode overrideReloadMode, initialDocument string) *overrideListenerFixture {
	data := mockld.NewServerSDKDataBuilder().
		Flag(
			dependencyFlag(depFlagTop, 1, depFlagLeaf),
			dependencyFlag(depFlagLeaf, 1, ""),
			segmentRuleFlag(depFlagSegment, depSegment),
			dependencyFlag(depFlagSegmentPrereq, 1, depFlagSegment),
			dependencyFlag(ovlFlagPlain, 1, ""),
		).
		Segment(dependencySegment(depSegment, 1, "")).
		Build()
	dataSystem := NewSDKDataSystem(t, data)
	file := NewOverrideFile(t, initialDocument)
	client := NewSDKClient(t, dataSystem, WithFileOverrides(mode.makeParams(file.Path)))
	return &overrideListenerFixture{streamPusher: newStreamPusher(dataSystem), client: client, file: file}
}

// Override documents used by the tests.

const ovlDocEmpty = "{}"

// ovlDocPlainValue overrides plain with a value.
func ovlDocPlainValue() string {
	return overrideDocument{FlagValues: map[string]ldvalue.Value{ovlFlagPlain: ovlOverrideValue}}.String()
}

// ovlDocPlainOther overrides plain with a different value.
func ovlDocPlainOther() string {
	return overrideDocument{FlagValues: map[string]ldvalue.Value{ovlFlagPlain: ldvalue.String("other-value")}}.String()
}

// ovlDocLeafValue overrides leaf, the prerequisite of top, with a value.
func ovlDocLeafValue() string {
	return overrideDocument{FlagValues: map[string]ldvalue.Value{depFlagLeaf: ovlOverrideValue}}.String()
}

// ovlDocSegmentIncludes overrides the segment with one that includes the test context.
func ovlDocSegmentIncludes() string {
	return overrideDocument{Segments: map[string]ldmodel.Segment{
		depSegment: dependencySegment(depSegment, 1, "", depContextKey),
	}}.String()
}

// ovlDocLeafServingOff overrides leaf with a flag that serves its variation 1, so top's
// prerequisite fails and top serves its off variation.
func ovlDocLeafServingOff() string {
	return overrideDocument{Flags: map[string]ldmodel.FeatureFlag{
		depFlagLeaf: dependencyFlagServing(depFlagLeaf, 1, "", depValueOffIndex),
	}}.String()
}

// ovlDocOverrideDependent adds an override flag that depends on the LaunchDarkly flag leaf.
func ovlDocOverrideDependent() string {
	return overrideDocument{Flags: map[string]ldmodel.FeatureFlag{
		ovlFlagOverrideDependent: dependencyFlag(ovlFlagOverrideDependent, 1, depFlagLeaf),
	}}.String()
}

// ovlDocOverrideSegmentFlag adds an override flag with a rule on the LaunchDarkly segment.
func ovlDocOverrideSegmentFlag() string {
	return overrideDocument{Flags: map[string]ldmodel.FeatureFlag{
		ovlFlagOverrideSegment: segmentRuleFlag(ovlFlagOverrideSegment, depSegment),
	}}.String()
}

// ovlDocOverrideViaPrereq adds an override flag whose prerequisite is the LaunchDarkly flag whose
// rule matches the LaunchDarkly segment, so a segment change reaches it through two LaunchDarkly
// items.
func ovlDocOverrideViaPrereq() string {
	return overrideDocument{Flags: map[string]ldmodel.FeatureFlag{
		ovlFlagOverrideViaPrereq: dependencyFlag(ovlFlagOverrideViaPrereq, 1, depFlagSegment),
	}}.String()
}

func doServerSideFlagOverridesListenerTests(t *ldtest.T) {
	t.Run("flag change listener", doServerSideFlagOverridesFlagChangeListenerTests)
	t.Run("flag value change listener", doServerSideFlagOverridesValueChangeListenerTests)
}

func doServerSideFlagOverridesFlagChangeListenerTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityFlagChangeListeners)

	for _, mode := range overrideReloadModes() {
		t.Run(mode.name, func(t *ldtest.T) {
			t.Run("adding an override notifies the flag", func(t *ldtest.T) {
				f := newOverrideListenerFixture(t, mode, ovlDocEmpty)
				callback := registerFlagChangeListener(t, f.client)

				f.file.Replace(t, ovlDocPlainValue())

				callback.ExpectFlagChangeNotifications(t, ovlFlagPlain)
			})

			t.Run("changing an override notifies the flag", func(t *ldtest.T) {
				f := newOverrideListenerFixture(t, mode, ovlDocPlainValue())
				callback := registerFlagChangeListener(t, f.client)

				f.file.Replace(t, ovlDocPlainOther())

				callback.ExpectFlagChangeNotifications(t, ovlFlagPlain)
			})

			t.Run("removing an override notifies the flag", func(t *ldtest.T) {
				f := newOverrideListenerFixture(t, mode, ovlDocPlainValue())
				callback := registerFlagChangeListener(t, f.client)

				f.file.Clear(t)

				callback.ExpectFlagChangeNotifications(t, ovlFlagPlain)
			})

			t.Run("deleting the file notifies the flags it overrode", func(t *ldtest.T) {
				f := newOverrideListenerFixture(t, mode, ovlDocPlainValue())
				callback := registerFlagChangeListener(t, f.client)

				f.file.Delete(t)

				callback.ExpectFlagChangeNotifications(t, ovlFlagPlain)
			})

			t.Run("rewriting an unchanged document notifies nothing", func(t *ldtest.T) {
				f := newOverrideListenerFixture(t, mode, ovlDocPlainValue())
				callback := registerFlagChangeListener(t, f.client)

				f.file.Replace(t, ovlDocPlainValue())

				// The wait must outlast the mode's reload latency, or the SDK could notify after the
				// test has already passed.
				callback.ExpectNoNotificationFor(t, ovlFlagPlain, mode.noticeWithin+listenerNoNotificationTimeout)
			})

			t.Run("overriding a prerequisite notifies the flags that depend on it", func(t *ldtest.T) {
				f := newOverrideListenerFixture(t, mode, ovlDocEmpty)
				callback := registerFlagChangeListener(t, f.client)

				f.file.Replace(t, ovlDocLeafValue())

				callback.ExpectFlagChangeNotifications(t, depFlagLeaf, depFlagTop)
			})

			t.Run("removing a prerequisite override notifies the flags that depend on it", func(t *ldtest.T) {
				f := newOverrideListenerFixture(t, mode, ovlDocLeafValue())
				callback := registerFlagChangeListener(t, f.client)

				f.file.Clear(t)

				callback.ExpectFlagChangeNotifications(t, depFlagLeaf, depFlagTop)
			})

			t.Run("overriding a segment notifies the flags whose rules reference it and the flags that depend on those",
				func(t *ldtest.T) {
					f := newOverrideListenerFixture(t, mode, ovlDocEmpty)
					callback := registerFlagChangeListener(t, f.client)

					f.file.Replace(t, ovlDocSegmentIncludes())

					callback.ExpectFlagChangeNotifications(t, depFlagSegment, depFlagSegmentPrereq)
				})

			t.Run("a LaunchDarkly change to a prerequisite notifies the override flag that depends on it", func(t *ldtest.T) {
				f := newOverrideListenerFixture(t, mode, ovlDocOverrideDependent())
				callback := registerFlagChangeListener(t, f.client)

				f.putFlag(dependencyFlag(depFlagLeaf, f.nextVersion(), ""))

				callback.ExpectFlagChangeNotifications(t, depFlagLeaf, depFlagTop, ovlFlagOverrideDependent)
			})

			t.Run("a LaunchDarkly change to a segment notifies the override flag whose rule references it", func(t *ldtest.T) {
				f := newOverrideListenerFixture(t, mode, ovlDocOverrideSegmentFlag())
				callback := registerFlagChangeListener(t, f.client)

				f.putSegment(dependencySegment(depSegment, f.nextVersion(), "", depContextKey))

				callback.ExpectFlagChangeNotifications(t, depFlagSegment, depFlagSegmentPrereq, ovlFlagOverrideSegment)
			})

			t.Run("a LaunchDarkly change to a segment notifies the override flag whose prerequisite's rule references it",
				func(t *ldtest.T) {
					f := newOverrideListenerFixture(t, mode, ovlDocOverrideViaPrereq())
					callback := registerFlagChangeListener(t, f.client)

					f.putSegment(dependencySegment(depSegment, f.nextVersion(), "", depContextKey))

					callback.ExpectFlagChangeNotifications(t, depFlagSegment, depFlagSegmentPrereq, ovlFlagOverrideViaPrereq)
				})
		})
	}
}

func doServerSideFlagOverridesValueChangeListenerTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityFlagValueChangeListeners)
	context := ldcontext.New(depContextKey)

	for _, mode := range overrideReloadModes() {
		t.Run(mode.name, func(t *ldtest.T) {
			t.Run("adding and removing an override reports the old and new values", func(t *ldtest.T) {
				f := newOverrideListenerFixture(t, mode, ovlDocEmpty)
				callback := registerFlagValueChangeListener(t, f.client, ovlFlagPlain, context, depDefaultValue)

				f.file.Replace(t, ovlDocPlainValue())
				callback.ExpectValueChangeNotification(t, ovlFlagPlain, depValueOn, ovlOverrideValue)

				f.file.Clear(t)
				callback.ExpectValueChangeNotification(t, ovlFlagPlain, ovlOverrideValue, depValueOn)
			})

			t.Run("overriding a prerequisite reports the new value of the flag that depends on it", func(t *ldtest.T) {
				f := newOverrideListenerFixture(t, mode, ovlDocEmpty)
				callback := registerFlagValueChangeListener(t, f.client, depFlagTop, context, depDefaultValue)

				f.file.Replace(t, ovlDocLeafServingOff())

				callback.ExpectValueChangeNotification(t, depFlagTop, depValueOn, depValueOff)
			})

			t.Run("overriding a segment reports the new value of a flag whose rule references it", func(t *ldtest.T) {
				f := newOverrideListenerFixture(t, mode, ovlDocEmpty)
				callback := registerFlagValueChangeListener(t, f.client, depFlagSegment, context, depDefaultValue)

				f.file.Replace(t, ovlDocSegmentIncludes())

				callback.ExpectValueChangeNotification(t, depFlagSegment, depValueNotIncluded, depValueIncluded)
			})

			t.Run("overriding a segment reports the new value of a flag whose prerequisite's rule references it",
				func(t *ldtest.T) {
					f := newOverrideListenerFixture(t, mode, ovlDocEmpty)
					callback := registerFlagValueChangeListener(t, f.client, depFlagSegmentPrereq, context, depDefaultValue)

					// segment-flag now serves "included", its variation 1, so the prerequisite on its variation 0 fails.
					f.file.Replace(t, ovlDocSegmentIncludes())

					callback.ExpectValueChangeNotification(t, depFlagSegmentPrereq, depValueOn, depValueOff)
				})

			t.Run("LaunchDarkly segment change reports the new value of an override flag depending on it through a prerequisite",
				func(t *ldtest.T) {
					f := newOverrideListenerFixture(t, mode, ovlDocOverrideViaPrereq())
					callback := registerFlagValueChangeListener(t, f.client, ovlFlagOverrideViaPrereq, context, depDefaultValue)

					f.putSegment(dependencySegment(depSegment, f.nextVersion(), "", depContextKey))

					callback.ExpectValueChangeNotification(t, ovlFlagOverrideViaPrereq, depValueOn, depValueOff)
				})

			t.Run("a LaunchDarkly change to a prerequisite reports the new value of the override flag that depends on it",
				func(t *ldtest.T) {
					f := newOverrideListenerFixture(t, mode, ovlDocOverrideDependent())
					callback := registerFlagValueChangeListener(t, f.client, ovlFlagOverrideDependent, context, depDefaultValue)

					f.putFlag(dependencyFlagServing(depFlagLeaf, f.nextVersion(), "", depValueOffIndex))

					callback.ExpectValueChangeNotification(t, ovlFlagOverrideDependent, depValueOn, depValueOff)
				})
		})
	}
}
