package mockld

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/launchdarkly/sdk-test-harness/framework"

	"github.com/gorilla/mux"
)

const (
	PollingPathServerSide      = "/sdk/latest-all"
	PollingPathMobileGet       = "/msdk/evalx/users/{user}"
	PollingPathMobileReport    = "/msdk/evalx/user"
	PollingPathJSClientGet     = "/sdk/evalx/{env}/users/{user}"
	PollingPathJSClientReport  = "/sdk/evalx/{env}/user"
	PollingPathPHPAllFlags     = "/sdk/flags"
	PollingPathPHPFlag         = "/sdk/flags/{key}"
	PollingPathPHPSegment      = "/sdk/segments/{key}"
	PollingPathUserBase64Param = "{user}"
	PollingPathEnvIDParam      = "{env}"
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

	pollHandler := p.standardPollingHandler()
	router := mux.NewRouter()
	switch sdkKind {
	case ServerSideSDK:
		router.Handle(PollingPathServerSide, pollHandler).Methods("GET")
	case RokuSDK:
		fallthrough
	case MobileSDK:
		router.Handle(PollingPathMobileGet, pollHandler).Methods("GET")
		router.Handle(PollingPathMobileReport, pollHandler).Methods("REPORT")
		// Note that we only support the "evalx", not the older "eval" which is used only by old unsupported SDKs
	case JSClientSDK:
		router.Handle(PollingPathJSClientGet, pollHandler).Methods("GET")
		router.Handle(PollingPathJSClientReport, pollHandler).Methods("REPORT")
	case PHPSDK:
		router.Handle(PollingPathPHPFlag, p.phpFlagHandler()).Methods("GET")
		router.Handle(PollingPathPHPSegment, p.phpSegmentHandler()).Methods("GET")
		router.Handle(PollingPathPHPAllFlags, p.phpAllFlagsHandler()).Methods("GET")
	}
	p.handler = router

	return p
}

func (p *PollingService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.handler.ServeHTTP(w, r)
}

func (p *PollingService) pollingHandler(getDataFn func(*PollingService, *http.Request) []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.lock.Lock()
		etag := p.currentEtag
		if matchEtag := r.Header.Get("If-None-Match"); matchEtag != "" && matchEtag == etag {
			p.lock.Unlock()
			w.WriteHeader(http.StatusNotModified)
			return
		}

		if p.currentData == nil || p.currentData.Serialize() == nil {
			// This means we've deliberately configured the data source to be unavailable
			w.WriteHeader(http.StatusNotFound)
			return
		}
		data := getDataFn(p, r)
		p.lock.Unlock()
		if data == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		p.debugLogger.Printf("Sending poll data for %s: %s", r.URL.Path, string(data))

		w.Header().Add("Content-Type", "application/json")
		if etag != "" {
			w.Header().Add("Etag", etag)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})
}

func (p *PollingService) standardPollingHandler() http.Handler {
	return p.pollingHandler(func(p *PollingService, r *http.Request) []byte {
		return p.currentData.Serialize()
	})
}

func (p *PollingService) phpFlagHandler() http.Handler {
	return p.pollingHandler(func(p *PollingService, r *http.Request) []byte {
		data, _ := p.currentData.(ServerSDKData)
		return data["flags"][mux.Vars(r)["key"]]
	})
}

func (p *PollingService) phpSegmentHandler() http.Handler {
	return p.pollingHandler(func(p *PollingService, r *http.Request) []byte {
		data, _ := p.currentData.(ServerSDKData)
		return data["segments"][mux.Vars(r)["key"]]
	})
}

func (p *PollingService) phpAllFlagsHandler() http.Handler {
	return p.pollingHandler(func(p *PollingService, r *http.Request) []byte {
		data, _ := p.currentData.(ServerSDKData)
		flagsJSON, _ := json.Marshal(data["flags"])
		return flagsJSON
	})
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
