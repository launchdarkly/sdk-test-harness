package data

import (
	"testing"
	"time"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldcontext"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldvalue"
	evaluation "gopkg.in/launchdarkly/go-server-sdk-evaluation.v2"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v2/ldbuilders"

	"github.com/stretchr/testify/assert"
)

func setFlagSalt(value string) func(*ldbuilders.FlagBuilder) {
	return func(b *ldbuilders.FlagBuilder) { b.Salt(value) }
}

func TestFlagFactory(t *testing.T) {
	t.Run("with value factory by type", func(t *testing.T) {
		factory := NewFlagFactory(
			"abc",
			MakeValueFactoryBySDKValueType(),
			setFlagSalt("x"),
		)
		usedKeys := make(map[string]bool)
		for _, valueType := range AllSDKValueTypes() {
			flag := factory.MakeFlagForValueType(valueType)
			assert.NotContains(t, usedKeys, flag.Key)
			m.In(t).Assert(flag.Key, m.StringHasPrefix("abc"))
			usedKeys[flag.Key] = true
			assertJSONValueTypeMatchesSDKValueType(t, flag.Variations[0], valueType)
			assert.Equal(t, "x", flag.Salt)
		}
	})

	t.Run("reuse previously created flag", func(t *testing.T) {
		factory1 := NewFlagFactory("abc", MakeValueFactoryBySDKValueType())
		for _, valueType := range AllSDKValueTypes() {
			flag1 := factory1.MakeFlagForValueType(valueType)
			assertJSONValueTypeMatchesSDKValueType(t, flag1.Variations[0], valueType)
			flag2 := factory1.ReuseFlagForValueType(valueType)
			assert.Equal(t, flag1, flag2)
		}

		for _, valueType := range AllSDKValueTypes() {
			factory2 := NewFlagFactory("abc", MakeValueFactoryBySDKValueType()) // has no previous flags
			flag1 := factory2.ReuseFlagForValueType(valueType)                  // lazily creates it
			assertJSONValueTypeMatchesSDKValueType(t, flag1.Variations[0], valueType)
			flag2 := factory2.ReuseFlagForValueType(valueType)
			assert.Equal(t, flag1, flag2)
		}
	})

	t.Run("make single flags", func(t *testing.T) {
		value := ldvalue.String("value")
		factory := NewFlagFactory(
			"abc",
			SingleValueForAllSDKValueTypes(value),
			setFlagSalt("x"),
		)

		flag1 := factory.MakeFlag()
		m.In(t).Assert(flag1.Variations[0], equalsLDValue(value))
		assert.Equal(t, "x", flag1.Salt)

		flag2 := factory.MakeFlag()
		m.In(t).Assert(flag2.Key, m.Not(m.Equal(flag1.Key)))
		m.In(t).Assert(flag2.Variations[0], equalsLDValue(value))
		assert.Equal(t, "x", flag1.Salt)
	})
}

func TestFlagShouldAlwaysHaveDebuggingEnabled(t *testing.T) {
	b := ldbuilders.NewFlagBuilder("key")
	FlagShouldAlwaysHaveDebuggingEnabled(b)
	assert.Greater(t, b.Build().DebugEventsUntilDate, ldtime.UnixMillisNow())
}

func TestFlagShouldHaveDebuggingEnabledUntil(t *testing.T) {
	timestamp := time.Now().Add(time.Hour)
	b := ldbuilders.NewFlagBuilder("key")
	FlagShouldHaveDebuggingEnabledUntil(timestamp)(b)
	assert.Equal(t, ldtime.UnixMillisFromTime(timestamp), b.Build().DebugEventsUntilDate)
}

func TestFlagShouldHaveFullEventTracking(t *testing.T) {
	b := ldbuilders.NewFlagBuilder("key")
	FlagShouldHaveFullEventTracking(b)
	assert.True(t, b.Build().TrackEvents)
}

func TestFlagShouldProduceThisEvalReason(t *testing.T) {
	for _, p := range []struct {
		name                string
		reason              ldreason.EvaluationReason
		shouldMatch         []ldcontext.Context
		doNotSpecifyMatches bool
		shouldNotMatch      []ldcontext.Context
	}{
		{
			name:        "off",
			reason:      ldreason.NewEvalReasonOff(),
			shouldMatch: []ldcontext.Context{ldcontext.New("any-key")},
		},
		{
			name:        "fallthrough",
			reason:      ldreason.NewEvalReasonFallthrough(),
			shouldMatch: []ldcontext.Context{ldcontext.New("any-key")},
		},
		{
			name:           "target match",
			reason:         ldreason.NewEvalReasonTargetMatch(),
			shouldMatch:    []ldcontext.Context{ldcontext.New("x"), ldcontext.New("y")},
			shouldNotMatch: []ldcontext.Context{ldcontext.New("z")},
		},
		{
			name:           "target match with non-default kind",
			reason:         ldreason.NewEvalReasonTargetMatch(),
			shouldMatch:    []ldcontext.Context{ldcontext.NewWithKind("org", "x"), ldcontext.NewWithKind("org", "y")},
			shouldNotMatch: []ldcontext.Context{ldcontext.New("x"), ldcontext.NewWithKind("org", "z")},
		},
		{
			name:           "rule match for specific contexts",
			reason:         ldreason.NewEvalReasonRuleMatch(0, "my-rule"),
			shouldMatch:    []ldcontext.Context{ldcontext.NewWithKind("org", "x"), ldcontext.NewWithKind("org", "y")},
			shouldNotMatch: []ldcontext.Context{ldcontext.New("x"), ldcontext.NewWithKind("org", "z")},
		},
		{
			name:                "rule match for all contexts",
			reason:              ldreason.NewEvalReasonRuleMatch(0, "my-rule"),
			shouldMatch:         []ldcontext.Context{ldcontext.NewWithKind("org", "x"), ldcontext.NewWithKind("org", "y")},
			doNotSpecifyMatches: true,
		},
		{
			name:           "rule match with nonzero index",
			reason:         ldreason.NewEvalReasonRuleMatch(2, "my-rule"),
			shouldMatch:    []ldcontext.Context{ldcontext.NewWithKind("org", "x"), ldcontext.NewWithKind("org", "y")},
			shouldNotMatch: []ldcontext.Context{ldcontext.New("x"), ldcontext.NewWithKind("org", "z")},
		},
		{
			name:        "error",
			reason:      ldreason.NewEvalReasonError(ldreason.EvalErrorMalformedFlag),
			shouldMatch: []ldcontext.Context{ldcontext.New("any-key")},
		},
	} {
		t.Run(p.name, func(t *testing.T) {
			b := ldbuilders.NewFlagBuilder("key").Variations(ldvalue.String("value"))
			shouldMatchParam := p.shouldMatch
			if p.doNotSpecifyMatches {
				shouldMatchParam = nil
			}
			FlagShouldProduceThisEvalReason(p.reason, shouldMatchParam...)(b)
			flag := b.Build()
			evaluator := evaluation.NewEvaluator(nil)
			for _, c := range p.shouldMatch {
				result := evaluator.Evaluate(&flag, c, nil)
				m.In(t).For(c.String()).Assert(result.Detail.Reason, m.JSONEqual(p.reason))
			}
			for _, c := range p.shouldNotMatch {
				result := evaluator.Evaluate(&flag, c, nil)
				m.In(t).For(c.String()).Assert(result.Detail.Reason, m.Not(m.JSONEqual(p.reason)))
			}
		})
	}
}
