package mockld

import (
	"compress/gzip"
	"encoding/json"
	"net/http"
	"strings"
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

	PollingPathPHPAllFlags = "/sdk/flags"
	PollingPathPHPFlag     = "/sdk/flags/{key}"
	PollingPathPHPSegment  = "/sdk/segments/{key}"

	PollingPathContextBase64Param = "{context}"
	PollingPathEnvIDParam         = "{env}"
)

type PollingService struct {
	sdkKind               SDKKind
	currentData           SDKData
	currentEtag           string
	handler               http.Handler
	enableGzipCompression bool
	debugLogger           framework.Logger
	lock                  sync.RWMutex
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
		router.Handle(PollingPathMobileGetUser, pollHandler).Methods("GET")
		router.Handle(PollingPathMobileReportUser, pollHandler).Methods("REPORT")
		// Note that we only support the "evalx", not the older "eval" which is used only by old unsupported SDKs
	case JSClientSDK:
		router.Handle(PollingPathJSClientGet, pollHandler).Methods("GET")
		router.Handle(PollingPathJSClientReport, pollHandler).Methods("REPORT")
		router.Handle(PollingPathJSClientGetUser, pollHandler).Methods("GET")
		router.Handle(PollingPathJSClientReportUser, pollHandler).Methods("REPORT")
	case PHPSDK:
		router.Handle(PollingPathPHPFlag, p.phpFlagHandler()).Methods("GET")
		router.Handle(PollingPathPHPSegment, p.phpSegmentHandler()).Methods("GET")
		router.Handle(PollingPathPHPAllFlags, p.phpAllFlagsHandler()).Methods("GET")
	}
	p.handler = router

	return p
}

func (p *PollingService) WithGzipCompression(enable bool) *PollingService {
	p.enableGzipCompression = enable
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

		switch {
		case p.enableGzipCompression && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip"):
			w.Header().Add("Content-Encoding", "gzip")
			w.WriteHeader(http.StatusOK)
			gzipWriter := gzip.NewWriter(w)
			if _, err := gzipWriter.Write(data); err != nil {
				p.debugLogger.Printf("failed to write to polling body gzip writer: %v", err)
			}
			if err := gzipWriter.Close(); err != nil {
				p.debugLogger.Printf("failed to flush gzip writer stream: %v", err)
			}

		case p.enableGzipCompression:
			w.WriteHeader(http.StatusBadRequest)
			p.debugLogger.Printf("gzip compression was enabled, but the required accept-encoding header was not set.")
		default:
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write(data); err != nil {
				p.debugLogger.Printf("failed to write polling body to writer: %v", err)
			}
		}
	})
}

func (p *PollingService) standardPollingHandler() http.Handler {
	return p.pollingHandler(func(p *PollingService, r *http.Request) []byte {
		fdv2SdkData, ok := p.currentData.(FDv2SDKData)
		if !ok {
			p.debugLogger.Println("poller cannot handle non-fdv2 sdk data at this time")
			return nil
		}

		events := make([]framework.PayloadEvent, 0, len(fdv2SdkData.events)+2)

		events = append(events, framework.PayloadEvent{
			Name: "server-intent",
			EventData: framework.ServerIntent{
				Payloads: []framework.Payload{
					{
						ID:     "payloadID",
						Target: 1,
						Code:   fdv2SdkData.intentCode,
						Reason: fdv2SdkData.intentReason,
					},
				},
			}})

		for _, obj := range fdv2SdkData.events {
			events = append(events, framework.PayloadEvent{
				Name:      "put-object",
				EventData: obj,
			})
		}

		events = append(events, framework.PayloadEvent{
			Name: "payload-transferred",
			EventData: framework.PayloadTransferred{
				State:   fdv2SdkData.state,
				Version: 1,
			},
		})

		payload := framework.PollingPayload{
			Events: events,
		}

		data, err := json.Marshal(payload)
		if err != nil {
			p.debugLogger.Printf("failed to marshal polling data: %v", err)
			return nil
		}

		return data
	})
}

func (p *PollingService) phpFlagHandler() http.Handler {
	return p.pollingHandler(func(p *PollingService, r *http.Request) []byte {
		//nolint:godox
		// TODO: Update this logic
		return []byte("UNSUPPORTED")
		// data, _ := p.currentData.(ServerSDKData)
		// return data["flags"][mux.Vars(r)["key"]]
	})
}

func (p *PollingService) phpSegmentHandler() http.Handler {
	return p.pollingHandler(func(p *PollingService, r *http.Request) []byte {
		//nolint:godox
		// TODO: Update this logic
		return []byte("UNSUPPORTED")
		// data, _ := p.currentData.(ServerSDKData)
		// return data["segments"][mux.Vars(r)["key"]]
	})
}

func (p *PollingService) phpAllFlagsHandler() http.Handler {
	return p.pollingHandler(func(p *PollingService, r *http.Request) []byte {
		//nolint:godox
		// TODO: Update this logic
		return []byte("UNSUPPORTED")
		// data, _ := p.currentData.(ServerSDKData)
		// flagsJSON, _ := json.Marshal(data["flags"])
		// return flagsJSON
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
