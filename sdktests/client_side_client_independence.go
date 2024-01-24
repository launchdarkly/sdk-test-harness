package sdktests

import (
	"sync"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/require"
)

func doClientSideClientIndependenceTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityClientIndependence)
	t.Run("same environments evaluate same", doClientSideClientIndependenceTestsSameEnvironment)
	t.Run("different environments evaluate independently", doClientSideClientIndependenceTestsMultipleEnvironmentsIndependent)
	t.Run("client A manipulated, client B unaffected", doClientSideClientIndependenceTestsClientAManipulatedClientBUnaffected)
}

// This test verifies two clients on the same environment have the same evaluations
func doClientSideClientIndependenceTestsSameEnvironment(t *ldtest.T) {
	contextA1 := ldcontext.New("user-a1")
	contextB1 := ldcontext.New("user-b1")
	flag1Key := "flag1"
	flag1Result1 := mockld.ClientSDKFlag{
		Value:     ldvalue.String("value1-a"),
		Variation: o.Some(0),
		Version:   1,
	}
	flag1Result2 := mockld.ClientSDKFlag{
		Value:     ldvalue.String("value1-b"),
		Variation: o.Some(1),
		Version:   2,
	}
	default1 := ldvalue.String("default1")

	dataBuilder := mockld.NewClientSDKDataBuilder().Flag(flag1Key, flag1Result1).Build()
	dataSource := NewSDKDataSource(t, dataBuilder)
	events := NewSDKEventSink(t)
	clientA := NewSDKClient(t,
		WithClientSideInitialContext(contextA1),
		dataSource, events)

	clientB := NewSDKClient(t,
		WithClientSideInitialContext(contextB1),
		dataSource, events)

	clientA.FlushEvents(t)
	clientB.FlushEvents(t)

	// wait for identify
	_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout) // discard initial identify event

	// check evaluations
	resp := clientA.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flag1Key,
		DefaultValue: default1,
	})

	if !m.In(t).Assert(flag1Result1.Value, m.JSONEqual(resp.Value)) {
		require.Fail(t, "evaluation unexpectedly returned wrong value")
	}

	resp = clientB.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flag1Key,
		DefaultValue: default1,
	})

	if !m.In(t).Assert(flag1Result1.Value, m.JSONEqual(resp.Value)) {
		require.Fail(t, "evaluation unexpectedly returned wrong value")
	}

	// change data from data source
	dataBuilderFlag1Result2 := mockld.NewClientSDKDataBuilder().Flag(flag1Key, flag1Result2).Build()
	dataSource.SetInitialData(dataBuilderFlag1Result2)
	clientA.SendIdentifyEvent(t, contextA1)
	clientB.SendIdentifyEvent(t, contextB1)

	// check evaluations
	resp = clientA.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flag1Key,
		DefaultValue: default1,
	})

	if !m.In(t).Assert(flag1Result2.Value, m.JSONEqual(resp.Value)) {
		require.Fail(t, "evaluation unexpectedly returned wrong value")
	}

	resp = clientB.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flag1Key,
		DefaultValue: default1,
	})

	if !m.In(t).Assert(flag1Result2.Value, m.JSONEqual(resp.Value)) {
		require.Fail(t, "evaluation unexpectedly returned wrong value")
	}
}

// This test verifies that evaluations on two clients on different environments are independent
func doClientSideClientIndependenceTestsMultipleEnvironmentsIndependent(t *ldtest.T) {
	contextA1 := ldcontext.New("user-a1")
	contextB1 := ldcontext.New("user-b1")
	flag1Key := "flag1"
	flag1Result1 := mockld.ClientSDKFlag{
		Value:     ldvalue.String("value1-a"),
		Variation: o.Some(0),
		Version:   1,
	}
	flag2Key := "flag2"
	flag2Result1 := mockld.ClientSDKFlag{
		Value:     ldvalue.String("value2-a"),
		Variation: o.Some(10),
		Version:   1,
	}
	default1 := ldvalue.String("default1")
	default2 := ldvalue.String("default2")

	dataBuilderFlag1Result1 := mockld.NewClientSDKDataBuilder().Flag(flag1Key, flag1Result1).Build()
	dataSourceA := NewSDKDataSource(t, dataBuilderFlag1Result1)
	eventsA := NewSDKEventSink(t)
	clientA := NewSDKClient(t,
		WithClientSideInitialContext(contextA1),
		dataSourceA, eventsA)

	dataBuilderFlag2Result1 := mockld.NewClientSDKDataBuilder().Flag(flag2Key, flag2Result1).Build()
	dataSourceB := NewSDKDataSource(t, dataBuilderFlag2Result1)
	eventsB := NewSDKEventSink(t)
	clientB := NewSDKClient(t,
		WithClientSideInitialContext(contextB1),
		dataSourceB, eventsB)

	clientA.FlushEvents(t)
	clientB.FlushEvents(t)

	// wait for both clients to identify
	w := sync.WaitGroup{}
	w.Add(2)
	go func() {
		defer w.Done()
		_ = eventsA.ExpectAnalyticsEvents(t, defaultEventTimeout)
	}() // discard initial identify event
	go func() {
		defer w.Done()
		_ = eventsB.ExpectAnalyticsEvents(t, defaultEventTimeout)
	}() // discard initial identify event
	w.Wait()

	// check evaluations
	resp := clientA.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flag1Key,
		DefaultValue: default1,
	})

	if !m.In(t).Assert(flag1Result1.Value, m.JSONEqual(resp.Value)) {
		require.Fail(t, "evaluation unexpectedly returned wrong value")
	}

	resp = clientB.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flag2Key,
		DefaultValue: default2,
	})

	if !m.In(t).Assert(flag2Result1.Value, m.JSONEqual(resp.Value)) {
		require.Fail(t, "evaluation unexpectedly returned wrong value")
	}
}

// This test verifies that if Client A identifies or is closed, Client B is unaffected
func doClientSideClientIndependenceTestsClientAManipulatedClientBUnaffected(t *ldtest.T) {
	contextA1 := ldcontext.New("user-a1")
	contextA2 := ldcontext.New("user-a2")
	contextB1 := ldcontext.New("user-b1")
	flag1Key := "flag1"
	flag1Result1 := mockld.ClientSDKFlag{
		Value:     ldvalue.String("value1-1"),
		Variation: o.Some(0),
		Version:   1,
	}
	flag1Result2 := mockld.ClientSDKFlag{
		Value:     ldvalue.String("value1-2"),
		Variation: o.Some(1),
		Version:   2,
	}
	flag2Key := "flag2"
	flag2Result1 := mockld.ClientSDKFlag{
		Value:     ldvalue.String("value2-1"),
		Variation: o.Some(10),
		Version:   1,
	}
	default1 := ldvalue.String("default1")
	default2 := ldvalue.String("default2")

	dataBuilderFlag1Result1 := mockld.NewClientSDKDataBuilder().Flag(flag1Key, flag1Result1).Build()
	dataSourceA := NewSDKDataSource(t, dataBuilderFlag1Result1)
	eventsA := NewSDKEventSink(t)
	clientA := NewSDKClient(t,
		WithClientSideInitialContext(contextA1),
		dataSourceA, eventsA)

	dataBuilderFlag2Result1 := mockld.NewClientSDKDataBuilder().Flag(flag2Key, flag2Result1).Build()
	dataSourceB := NewSDKDataSource(t, dataBuilderFlag2Result1)
	eventsB := NewSDKEventSink(t)
	clientB := NewSDKClient(t,
		WithClientSideInitialContext(contextB1),
		dataSourceB, eventsB)

	clientA.FlushEvents(t)
	clientB.FlushEvents(t)

	// wait for both clients to identify
	w := sync.WaitGroup{}
	w.Add(2)
	go func() {
		defer w.Done()
		_ = eventsA.ExpectAnalyticsEvents(t, defaultEventTimeout)
	}() // discard initial identify event
	go func() {
		defer w.Done()
		_ = eventsB.ExpectAnalyticsEvents(t, defaultEventTimeout)
	}() // discard initial identify event
	w.Wait()

	// check client evaluations as reference point
	resp := clientA.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flag1Key,
		DefaultValue: default1,
	})

	if !m.In(t).Assert(flag1Result1.Value, m.JSONEqual(resp.Value)) {
		require.Fail(t, "evaluation unexpectedly returned wrong value")
	}

	resp = clientB.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flag2Key,
		DefaultValue: default2,
	})

	if !m.In(t).Assert(flag2Result1.Value, m.JSONEqual(resp.Value)) {
		require.Fail(t, "evaluation unexpectedly returned wrong value")
	}

	// now identify on A and verify event from A, but no event from B
	dataBuilderFlag1Result2 := mockld.NewClientSDKDataBuilder().Flag(flag1Key, flag1Result2).Build()
	dataSourceA.SetInitialData(dataBuilderFlag1Result2)
	clientA.SendIdentifyEvent(t, contextA2)

	// check that client A evaluates to a new value
	resp = clientA.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flag1Key,
		DefaultValue: default1,
	})

	if !m.In(t).Assert(flag1Result2.Value, m.JSONEqual(resp.Value)) {
		require.Fail(t, "evaluation unexpectedly returned wrong value")
	}

	// check that client B evaluates to the same value it did before
	resp = clientB.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flag2Key,
		DefaultValue: default2,
	})

	if !m.In(t).Assert(flag2Result1.Value, m.JSONEqual(resp.Value)) {
		require.Fail(t, "evaluation unexpectedly returned wrong value")
	}

	clientA.Close()
	// verify client B is unaffected by closing client A
	resp = clientB.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flag2Key,
		DefaultValue: default2,
	})

	if !m.In(t).Assert(flag2Result1.Value, m.JSONEqual(resp.Value)) {
		require.Fail(t, "evaluation unexpectedly returned wrong value")
	}
}
