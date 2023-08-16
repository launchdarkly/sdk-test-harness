package mockld

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/launchdarkly/sdk-test-harness/v2/framework"

	"github.com/gorilla/mux"
)

// callbackService provides support infrastructure for callback services that simulate an SDK
// component with multiple methods. The test service will call one of several predefined subpaths
// for each component method. For simplicity, and to make it clear that these calls should never
// be cached, the method is always POST, except for a DELETE method to the base path which means
// the SDK has stopped using the component. The parameters and responses, if any, are always JSON.
type callbackService struct {
	router *mux.Router
	logger framework.Logger
	name   string
}

func newCallbackService(logger framework.Logger, name string) *callbackService {
	router := mux.NewRouter()
	c := &callbackService{router: router, logger: logger, name: name}
	router.HandleFunc("/", c.close).Methods("DELETE")
	return c
}

func (c *callbackService) addPath(path string, handler func(*json.Decoder) (interface{}, error)) {
	c.router.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		var requestDecoder *json.Decoder
		var body []byte
		if r.Body != nil {
			body, _ = ioutil.ReadAll(r.Body)
			_ = r.Body.Close()
			requestDecoder = json.NewDecoder(bytes.NewBuffer(body))
		}
		c.logger.Printf("[%s] Got POST %s %s", c.name, path, string(body))

		responseValue, err := handler(requestDecoder)
		if err != nil {
			w.Header().Set("content-type", "text/plain")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			c.logger.Printf("[%s] Responded with 500 - %s", c.name, err.Error())
		} else {
			w.Header().Set("content-type", "application/json")
			var respBody []byte
			w.WriteHeader(http.StatusOK)
			if responseValue != nil {
				respBody, _ = json.Marshal(responseValue)
				_, _ = w.Write(respBody)
			}
			c.logger.Printf("[%s] Responded with 200 %s", c.name, string(respBody))
		}
	})
}

func (c *callbackService) close(w http.ResponseWriter, r *http.Request) {
	c.logger.Printf("[%s] Got DELETE - closing fixture", c.name)
	w.WriteHeader(http.StatusNoContent)
}
