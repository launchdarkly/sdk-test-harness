package sdktests

import (
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"

	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
)

// The dependency tests share one data set. An arrow points from an item to the item it depends on.
//
//	top -> mid -> leaf                        prerequisites
//	segment-flag -> segment                   flag rule with a segmentMatch clause
//	segment-prereq-flag -> segment-flag       prerequisite on the flag whose rule matches the segment
//	nested-segment-flag -> outer -> inner     flag rule on a segment whose own rule matches another segment
//	unrelated                                 no dependencies
//
// A change to an item must notify the flags that depend on it, directly or through the chain, and
// nothing else. The value tests use the same data: every flag serves "on-value" while its
// prerequisites are satisfied and "off-value" otherwise, and the segment flags serve "included"
// for a context that the segment includes and "not-included" for any other.
const (
	depFlagTop               = "top"
	depFlagMid               = "mid"
	depFlagLeaf              = "leaf"
	depFlagSegment           = "segment-flag"
	depFlagSegmentPrereq     = "segment-prereq-flag"
	depFlagNestedSegment     = "nested-segment-flag"
	depFlagUnrelated         = "unrelated"
	depSegment               = "segment"
	depSegmentOuter          = "outer"
	depSegmentInner          = "inner"
	depContextKey            = "user-key"
	depPayloadState          = "updated"
	depValueOnIndex          = 0
	depValueOffIndex         = 1
	depValueNotIncludedIndex = 0
	depValueIncludedIndex    = 1
)

var (
	depValueOn          = ldvalue.String("on-value")     //nolint:gochecknoglobals
	depValueOff         = ldvalue.String("off-value")    //nolint:gochecknoglobals
	depValueIncluded    = ldvalue.String("included")     //nolint:gochecknoglobals
	depValueNotIncluded = ldvalue.String("not-included") //nolint:gochecknoglobals
	depDefaultValue     = ldvalue.String("default")      //nolint:gochecknoglobals
)

// dependencyFlag builds a flag that is on and serves "on-value" by fallthrough, with "off-value"
// as its off variation. When prerequisite is not empty, the flag requires that flag to serve its
// variation 0, so the flag serves "off-value" whenever the prerequisite serves anything else or is
// missing.
func dependencyFlag(key string, version int, prerequisite string) ldmodel.FeatureFlag {
	return dependencyFlagServing(key, version, prerequisite, depValueOnIndex)
}

// dependencyFlagServing is dependencyFlag with the fallthrough variation chosen by the caller, so
// a prerequisite can be made to stop satisfying the flags that depend on it.
func dependencyFlagServing(key string, version int, prerequisite string, fallthroughVariation int) ldmodel.FeatureFlag {
	builder := ldbuilders.NewFlagBuilder(key).Version(version).On(true).
		FallthroughVariation(fallthroughVariation).OffVariation(depValueOffIndex).
		Variations(depValueOn, depValueOff)
	if prerequisite != "" {
		builder.AddPrerequisite(prerequisite, depValueOnIndex)
	}
	return builder.Build()
}

// segmentRuleFlag builds a flag, at version 1, that serves "included" for a context that the
// segment includes and "not-included" otherwise.
func segmentRuleFlag(key string, segmentKey string) ldmodel.FeatureFlag {
	return ldbuilders.NewFlagBuilder(key).Version(1).On(true).
		FallthroughVariation(depValueNotIncludedIndex).OffVariation(depValueNotIncludedIndex).
		Variations(depValueNotIncluded, depValueIncluded).
		AddRule(ldbuilders.NewRuleBuilder().ID("segment-rule").Variation(depValueIncludedIndex).
			Clauses(ldbuilders.SegmentMatchClause(segmentKey))).
		Build()
}

// dependencySegment builds a segment with the given included keys. When nested is not empty, the
// segment also has a rule that matches any context in that other segment.
func dependencySegment(key string, version int, nested string, included ...string) ldmodel.Segment {
	builder := ldbuilders.NewSegmentBuilder(key).Version(version).Included(included...)
	if nested != "" {
		builder.AddRule(ldbuilders.NewSegmentRuleBuilder().ID("nested-rule").
			Clauses(ldbuilders.SegmentMatchClause(nested)))
	}
	return builder.Build()
}

func dependencyFlags() []ldmodel.FeatureFlag {
	return []ldmodel.FeatureFlag{
		dependencyFlag(depFlagTop, 1, depFlagMid),
		dependencyFlag(depFlagMid, 1, depFlagLeaf),
		dependencyFlag(depFlagLeaf, 1, ""),
		segmentRuleFlag(depFlagSegment, depSegment),
		// segment-flag serves its variation 0 ("not-included") until the segment includes the context,
		// so this prerequisite is satisfied at first and fails once the segment changes.
		dependencyFlag(depFlagSegmentPrereq, 1, depFlagSegment),
		segmentRuleFlag(depFlagNestedSegment, depSegmentOuter),
		dependencyFlag(depFlagUnrelated, 1, ""),
	}
}

func dependencySegments() []ldmodel.Segment {
	return []ldmodel.Segment{
		dependencySegment(depSegment, 1, ""),
		dependencySegment(depSegmentOuter, 1, depSegmentInner),
		dependencySegment(depSegmentInner, 1, ""),
	}
}

// streamPusher pushes changes through an FDv2 streaming service. Every pushed item and every
// payload-transferred event carries a version higher than any used before, so the SDK never
// discards a push as stale.
type streamPusher struct {
	streaming *mockld.StreamingService
	version   int
}

func newStreamPusher(dataSystem *SDKDataSystem) *streamPusher {
	return &streamPusher{
		streaming: dataSystem.Synchronizers[0].streaming,
		version:   1, // the initial payload
	}
}

// nextVersion returns a version higher than any used so far. Use it for the item being pushed.
func (p *streamPusher) nextVersion() int {
	p.version++
	return p.version
}

// putFlag pushes the flag as a change to the current payload, using the flag's own version for
// the payload-transferred event. Build the flag with a version from nextVersion.
func (p *streamPusher) putFlag(flag ldmodel.FeatureFlag) {
	p.streaming.PushUpdate("flag", flag.Key, flag.Version, jsonhelpers.ToJSON(flag))
	p.streaming.PushPayloadTransferred(depPayloadState, flag.Version)
}

// putSegment is putFlag for a segment.
func (p *streamPusher) putSegment(segment ldmodel.Segment) {
	p.streaming.PushUpdate("segment", segment.Key, segment.Version, jsonhelpers.ToJSON(segment))
	p.streaming.PushPayloadTransferred(depPayloadState, segment.Version)
}

// deleteItem pushes a deletion of the flag or segment as a change to the current payload.
func (p *streamPusher) deleteItem(namespace, key string) {
	version := p.nextVersion()
	p.streaming.PushDelete(namespace, key, version)
	p.streaming.PushPayloadTransferred(depPayloadState, version)
}

// replaceAll pushes a full data set that replaces everything the SDK holds. An item that keeps
// its version is unchanged; an item that is absent is removed.
func (p *streamPusher) replaceAll(flags []ldmodel.FeatureFlag, segments []ldmodel.Segment) {
	p.streaming.PushServerIntent("xfer-full", "stale")
	for _, flag := range flags {
		p.streaming.PushUpdate("flag", flag.Key, flag.Version, jsonhelpers.ToJSON(flag))
	}
	for _, segment := range segments {
		p.streaming.PushUpdate("segment", segment.Key, segment.Version, jsonhelpers.ToJSON(segment))
	}
	p.streaming.PushPayloadTransferred(depPayloadState, p.nextVersion())
}

// dependencyListenerFixture is an SDK client started with the dependency data set, plus the
// pusher through which the tests change that data.
type dependencyListenerFixture struct {
	*streamPusher
	client *SDKClient
}

func newDependencyListenerFixture(t *ldtest.T) *dependencyListenerFixture {
	data := mockld.NewServerSDKDataBuilder().Flag(dependencyFlags()...).Segment(dependencySegments()...).Build()
	dataSystem := NewSDKDataSystem(t, data)
	client := NewSDKClient(t, dataSystem)
	return &dependencyListenerFixture{streamPusher: newStreamPusher(dataSystem), client: client}
}

// listenForFlagChanges registers a general flag change listener and returns its callback.
func (f *dependencyListenerFixture) listenForFlagChanges(t *ldtest.T) *ListenerCallback {
	return registerFlagChangeListener(t, f.client)
}

// listenForValueChanges registers a flag value change listener for one flag and returns its callback.
func (f *dependencyListenerFixture) listenForValueChanges(t *ldtest.T, flagKey string) *ListenerCallback {
	return registerFlagValueChangeListener(t, f.client, flagKey, ldcontext.New(depContextKey), depDefaultValue)
}

// withoutFlag returns the flags without the one with the given key.
func withoutFlag(flags []ldmodel.FeatureFlag, key string) []ldmodel.FeatureFlag {
	result := make([]ldmodel.FeatureFlag, 0, len(flags))
	for _, flag := range flags {
		if flag.Key != key {
			result = append(result, flag)
		}
	}
	return result
}

// replaceFlag returns the flags with the one with the same key as replacement swapped for it.
func replaceFlag(flags []ldmodel.FeatureFlag, replacement ldmodel.FeatureFlag) []ldmodel.FeatureFlag {
	result := make([]ldmodel.FeatureFlag, 0, len(flags))
	for _, flag := range flags {
		if flag.Key == replacement.Key {
			result = append(result, replacement)
		} else {
			result = append(result, flag)
		}
	}
	return result
}

// replaceSegment returns the segments with the one with the same key as replacement swapped for it.
func replaceSegment(segments []ldmodel.Segment, replacement ldmodel.Segment) []ldmodel.Segment {
	result := make([]ldmodel.Segment, 0, len(segments))
	for _, segment := range segments {
		if segment.Key == replacement.Key {
			result = append(result, replacement)
		} else {
			result = append(result, segment)
		}
	}
	return result
}

// doFlagChangeListenerDependencyTests checks that a general flag change listener hears about every
// flag whose configuration a change may have affected through prerequisites and segment references,
// and about nothing else.
func doFlagChangeListenerDependencyTests(t *ldtest.T) {
	t.Run("prerequisite change notifies the flags that depend on it", func(t *ldtest.T) {
		f := newDependencyListenerFixture(t)
		callback := f.listenForFlagChanges(t)

		f.putFlag(dependencyFlag(depFlagLeaf, f.nextVersion(), ""))

		callback.ExpectFlagChangeNotifications(t, depFlagLeaf, depFlagMid, depFlagTop)
	})

	t.Run("change in the middle of a prerequisite chain notifies only the flags above it", func(t *ldtest.T) {
		f := newDependencyListenerFixture(t)
		callback := f.listenForFlagChanges(t)

		f.putFlag(dependencyFlag(depFlagMid, f.nextVersion(), depFlagLeaf))

		callback.ExpectFlagChangeNotifications(t, depFlagMid, depFlagTop)
	})

	t.Run("prerequisite deletion notifies the flags that depend on it", func(t *ldtest.T) {
		f := newDependencyListenerFixture(t)
		callback := f.listenForFlagChanges(t)

		f.deleteItem("flag", depFlagLeaf)

		callback.ExpectFlagChangeNotifications(t, depFlagLeaf, depFlagMid, depFlagTop)
	})

	t.Run("segment change notifies the flags whose rules reference it and the flags that depend on those",
		func(t *ldtest.T) {
			f := newDependencyListenerFixture(t)
			callback := f.listenForFlagChanges(t)

			f.putSegment(dependencySegment(depSegment, f.nextVersion(), "", depContextKey))

			callback.ExpectFlagChangeNotifications(t, depFlagSegment, depFlagSegmentPrereq)
		})

	t.Run("segment deletion notifies the flags whose rules reference it and the flags that depend on those",
		func(t *ldtest.T) {
			f := newDependencyListenerFixture(t)
			callback := f.listenForFlagChanges(t)

			f.deleteItem("segment", depSegment)

			callback.ExpectFlagChangeNotifications(t, depFlagSegment, depFlagSegmentPrereq)
		})

	t.Run("nested segment change notifies the flags through the segment that references it", func(t *ldtest.T) {
		f := newDependencyListenerFixture(t)
		callback := f.listenForFlagChanges(t)

		f.putSegment(dependencySegment(depSegmentInner, f.nextVersion(), "", depContextKey))

		callback.ExpectFlagChangeNotifications(t, depFlagNestedSegment)
	})

	t.Run("a dependency added by an update is followed afterwards", func(t *ldtest.T) {
		f := newDependencyListenerFixture(t)
		callback := f.listenForFlagChanges(t)

		// The unrelated flag starts depending on leaf.
		f.putFlag(dependencyFlag(depFlagUnrelated, f.nextVersion(), depFlagLeaf))
		callback.ExpectFlagChangeNotifications(t, depFlagUnrelated)

		f.putFlag(dependencyFlag(depFlagLeaf, f.nextVersion(), ""))
		callback.ExpectFlagChangeNotifications(t, depFlagLeaf, depFlagMid, depFlagTop, depFlagUnrelated)
	})

	t.Run("a dependency removed by an update is not followed afterwards", func(t *ldtest.T) {
		f := newDependencyListenerFixture(t)
		callback := f.listenForFlagChanges(t)

		// mid stops depending on leaf, which cuts top off from leaf as well.
		f.putFlag(dependencyFlag(depFlagMid, f.nextVersion(), ""))
		callback.ExpectFlagChangeNotifications(t, depFlagMid, depFlagTop)

		f.putFlag(dependencyFlag(depFlagLeaf, f.nextVersion(), ""))
		callback.ExpectFlagChangeNotifications(t, depFlagLeaf)
	})

	t.Run("full data set notifies the dependents of each item whose version changed", func(t *ldtest.T) {
		f := newDependencyListenerFixture(t)
		callback := f.listenForFlagChanges(t)

		flags := replaceFlag(dependencyFlags(), dependencyFlag(depFlagLeaf, f.nextVersion(), ""))
		segments := replaceSegment(dependencySegments(), dependencySegment(depSegment, f.nextVersion(), "", depContextKey))
		f.replaceAll(flags, segments)

		callback.ExpectFlagChangeNotifications(t, depFlagLeaf, depFlagMid, depFlagTop, depFlagSegment, depFlagSegmentPrereq)
	})

	t.Run("full data set notifies the dependents of each item it no longer contains", func(t *ldtest.T) {
		f := newDependencyListenerFixture(t)
		callback := f.listenForFlagChanges(t)

		f.replaceAll(withoutFlag(dependencyFlags(), depFlagLeaf), dependencySegments())

		callback.ExpectFlagChangeNotifications(t, depFlagLeaf, depFlagMid, depFlagTop)
	})
}

// doFlagValueChangeListenerDependencyTests checks that a flag value change listener reports a new
// value that a change to a prerequisite or a segment produced, and stays quiet when such a change
// leaves the value as it was.
func doFlagValueChangeListenerDependencyTests(t *ldtest.T) {
	t.Run("prerequisite change that alters the value notifies", func(t *ldtest.T) {
		f := newDependencyListenerFixture(t)
		callback := f.listenForValueChanges(t, depFlagTop)

		// leaf now serves its variation 1, so mid's prerequisite fails, mid serves its off variation,
		// and top's prerequisite on mid fails in turn.
		f.putFlag(dependencyFlagServing(depFlagLeaf, f.nextVersion(), "", depValueOffIndex))

		callback.ExpectValueChangeNotification(t, depFlagTop, depValueOn, depValueOff)
	})

	t.Run("prerequisite change that leaves the value unchanged does not notify", func(t *ldtest.T) {
		f := newDependencyListenerFixture(t)
		callback := f.listenForValueChanges(t, depFlagTop)

		f.putFlag(dependencyFlag(depFlagLeaf, f.nextVersion(), ""))

		callback.ExpectNoNotification(t, depFlagTop)
	})

	t.Run("prerequisite deletion notifies with the off value", func(t *ldtest.T) {
		f := newDependencyListenerFixture(t)
		callback := f.listenForValueChanges(t, depFlagTop)

		f.deleteItem("flag", depFlagLeaf)

		callback.ExpectValueChangeNotification(t, depFlagTop, depValueOn, depValueOff)
	})

	t.Run("segment change that alters the value notifies", func(t *ldtest.T) {
		f := newDependencyListenerFixture(t)
		callback := f.listenForValueChanges(t, depFlagSegment)

		f.putSegment(dependencySegment(depSegment, f.nextVersion(), "", depContextKey))

		callback.ExpectValueChangeNotification(t, depFlagSegment, depValueNotIncluded, depValueIncluded)
	})

	t.Run("segment change that alters a prerequisite's value notifies the flag that depends on it", func(t *ldtest.T) {
		f := newDependencyListenerFixture(t)
		callback := f.listenForValueChanges(t, depFlagSegmentPrereq)

		// segment-flag now serves "included", its variation 1, so the prerequisite on its variation 0 fails.
		f.putSegment(dependencySegment(depSegment, f.nextVersion(), "", depContextKey))

		callback.ExpectValueChangeNotification(t, depFlagSegmentPrereq, depValueOn, depValueOff)
	})

	t.Run("segment change that leaves the value unchanged does not notify", func(t *ldtest.T) {
		f := newDependencyListenerFixture(t)
		callback := f.listenForValueChanges(t, depFlagSegment)

		f.putSegment(dependencySegment(depSegment, f.nextVersion(), "", "someone-else"))

		callback.ExpectNoNotification(t, depFlagSegment)
	})

	t.Run("nested segment change that alters the value notifies", func(t *ldtest.T) {
		f := newDependencyListenerFixture(t)
		callback := f.listenForValueChanges(t, depFlagNestedSegment)

		f.putSegment(dependencySegment(depSegmentInner, f.nextVersion(), "", depContextKey))

		callback.ExpectValueChangeNotification(t, depFlagNestedSegment, depValueNotIncluded, depValueIncluded)
	})
}
