package mockld

import (
	"net/http"
	"sync"

	"github.com/launchdarkly/sdk-test-harness/v2/framework"

	"github.com/gorilla/mux"
)

const (
	PollingPathServerSide     = "/sdk/latest-all"
	PollingPathMobileGet      = "/msdk/evalx/contexts/{context}"
	PollingPathMobileReport   = "/msdk/evalx/context"
	PollingPathJSClientGet    = "/sdk/evalx/{env}/contexts/{context}"
	PollingPathJSClientReport = "/sdk/evalx/{env}/context"

	// The following endpoint paths were used by older SDKs based on the user model rather than
	// the context model. New context-aware SDKs should always use the new paths. However, our
	// mock service still supports the old paths (just as the real LD services do). We have
	// specific tests to verify that the SDKs use the new paths; in all other tests, if the SDK
	// uses an old path, it will still work so that we don't confusingly see every test fail.
	// We do *not* support the very old "eval" (as opposed to "evalx") paths since the only SDKs
	// that used them are long past EOL.
	PollingPathMobileGetUser      = "/msdk/evalx/users/{context}"
	PollingPathMobileReportUser   = "/msdk/evalx/user"
	PollingPathJSClientGetUser    = "/sdk/evalx/{env}/users/{context}"
	PollingPathJSClientReportUser = "/sdk/evalx/{env}/user"

	PollingPathContextBase64Param = "{context}"
	PollingPathEnvIDParam         = "{env}"
)

type PollingService struct {
	sdkKind     SDKKind
	currentData SDKData
	currentEtag string
	handler     http.Handler
	debugLogger framework.Logger
	lock        sync.RWMutex
}

func NewPollingService(
	initialData SDKData,
	sdkKind SDKKind,
	debugLogger framework.Logger,
) *PollingService {
	p := &PollingService{
		sdkKind:     sdkKind,
		currentData: initialData,
		debugLogger: debugLogger,
	}

	pollHandler := p.servePollRequest
	router := mux.NewRouter()
	switch sdkKind {
	case ServerSideSDK:
		router.HandleFunc(PollingPathServerSide, pollHandler).Methods("GET")
	case MobileSDK:
		router.HandleFunc(PollingPathMobileGet, pollHandler).Methods("GET")
		router.HandleFunc(PollingPathMobileReport, pollHandler).Methods("REPORT")
		router.HandleFunc(PollingPathMobileGetUser, pollHandler).Methods("GET")
		router.HandleFunc(PollingPathMobileReportUser, pollHandler).Methods("REPORT")
	case JSClientSDK:
		router.HandleFunc(PollingPathJSClientGet, pollHandler).Methods("GET")
		router.HandleFunc(PollingPathJSClientReport, pollHandler).Methods("REPORT")
		router.HandleFunc(PollingPathJSClientGetUser, pollHandler).Methods("GET")
		router.HandleFunc(PollingPathJSClientReportUser, pollHandler).Methods("REPORT")
	}
	p.handler = router

	return p
}

func (p *PollingService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.handler.ServeHTTP(w, r)
}

func (p *PollingService) servePollRequest(w http.ResponseWriter, r *http.Request) {
	p.lock.Lock()
	etag := p.currentEtag
	if matchEtag := r.Header.Get("If-None-Match"); matchEtag != "" && matchEtag == etag {
		p.lock.Unlock()
		w.WriteHeader(http.StatusNotModified)
		return
	}
	data := p.currentData.Serialize()
	p.lock.Unlock()

	p.debugLogger.Printf("Sending poll data: %s", string(data))

	w.Header().Add("Content-Type", "application/json")
	if etag != "" {
		w.Header().Add("Etag", etag)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (p *PollingService) SetData(data SDKData) {
	p.lock.Lock()
	p.currentData = data
	p.lock.Unlock()
}

func (p *PollingService) SetEtag(etag string) {
	p.lock.Lock()
	p.currentEtag = etag
	p.lock.Unlock()
}
