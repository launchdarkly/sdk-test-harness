package mockld

import (
	"net/http"
	"sync"

	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/sdk-test-harness/v2/framework"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
)

type CallHistory struct {
	callOrder int
	origin    ldmigration.MigrationOrigin
}

func (c *CallHistory) GetOrigin() ldmigration.MigrationOrigin {
	return c.origin
}

func (c *CallHistory) GetCallOrder() int {
	return c.callOrder
}

type MigrationCallbackService struct {
	lock        sync.Mutex
	callHistory []CallHistory
	callCount   int

	oldEndpoint *harness.MockEndpoint
	newEndpoint *harness.MockEndpoint
}

func NewMigrationCallbackService(
	testHarness *harness.TestHarness,
	logger framework.Logger,
	oldHandler func(w http.ResponseWriter, req *http.Request),
	newHandler func(w http.ResponseWriter, req *http.Request),
) *MigrationCallbackService {
	m := &MigrationCallbackService{
		callHistory: make([]CallHistory, 0),
		callCount:   0,
	}

	oldReadHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		m.trackCallHistory(ldmigration.Old)
		oldHandler(w, req)
	})

	newReadHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		m.trackCallHistory(ldmigration.New)
		newHandler(w, req)
	})

	m.oldEndpoint = testHarness.NewMockEndpoint(oldReadHandler, logger, harness.MockEndpointDescription("old endpoint"))
	m.newEndpoint = testHarness.NewMockEndpoint(newReadHandler, logger, harness.MockEndpointDescription("new endpoint"))

	return m
}

func (m *MigrationCallbackService) trackCallHistory(origin ldmigration.MigrationOrigin) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.callCount += 1
	m.callHistory = append(m.callHistory, CallHistory{callOrder: m.callCount, origin: origin})
}

func (m *MigrationCallbackService) Close() {
	m.oldEndpoint.Close()
	m.newEndpoint.Close()
}

func (m *MigrationCallbackService) GetCallHistory() []CallHistory {
	return m.callHistory
}

func (m *MigrationCallbackService) OldEndpoint() *harness.MockEndpoint {
	return m.oldEndpoint
}

func (m *MigrationCallbackService) NewEndpoint() *harness.MockEndpoint {
	return m.newEndpoint
}
