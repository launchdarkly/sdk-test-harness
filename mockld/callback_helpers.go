package mockld

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/launchdarkly/sdk-test-harness/framework"

	"github.com/gorilla/mux"
)

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
		c.logger.Printf("[%s] got POST %s %s", c.name, path, string(body))

		responseValue, err := handler(requestDecoder)
		if err != nil {
			w.Header().Set("content-type", "text/plain")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			c.logger.Printf("[%s] responded with 500 - %s", c.name, err.Error())
		} else {
			w.Header().Set("content-type", "application/json")
			var respBody []byte
			w.WriteHeader(http.StatusOK)
			if responseValue != nil {
				respBody, _ = json.Marshal(responseValue)
				_, _ = w.Write(respBody)
			}
			c.logger.Printf("[%s] responded with 200 %s", c.name, string(respBody))
		}
	})
}

func (c *callbackService) close(w http.ResponseWriter, r *http.Request) {
	c.logger.Printf("[%s] got DELETE - closing fixture", c.name)
	w.WriteHeader(http.StatusNoContent)
}
