package mockld

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gorilla/mux"
)

// SDK expects these fields to be present, but it will ignore these fields when
// running in plain text mode against the test harness.
type handshakePublicBundle struct {
	AuthenticationKey string `json:"authenticationKey"`
	CipherKey         string `json:"cipherKey"`
	ServerBundle      string `json:"serverBundle"`
}

type RokuServer struct {
	user      *json.RawMessage
	mobileKey *string
}

// NewRokuStreamHandler returns a handler that answers the Roku handshake and sends Roku stream
// requests to h as mobile stream requests. All requests share one handshake state. Use this when h
// can be a different handler for each request, for example a sequential handler.
func NewRokuStreamHandler(h http.Handler) http.Handler {
	router := mux.NewRouter()
	(&RokuServer{}).Route(router, h)
	return router
}

// Route adds the Roku handshake and stream paths to router. Stream requests go to h.
func (srv *RokuServer) Route(router *mux.Router, h http.Handler) {
	router.Path(StreamingPathRokuHandshake).Methods("POST").HandlerFunc(srv.ServeHandshake)
	router.Path(StreamingPathRokuEvaluate).Methods("POST").Handler(srv.Wrap(h))
}

func (srv *RokuServer) Wrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(baseWriter http.ResponseWriter, req *http.Request) {
		userJSON, err := json.Marshal(srv.user)
		if err != nil {
			baseWriter.WriteHeader(http.StatusInternalServerError)
			return
		}

		userJSONBase64 := base64.URLEncoding.EncodeToString(userJSON)

		req.Method = "GET"
		req.Header.Set("Authorization", *srv.mobileKey)
		req.ContentLength = 0
		req.Body = nil

		req.URL.Path = "/meval/" + userJSONBase64

		h.ServeHTTP(baseWriter, req)
	})
}

func (srv *RokuServer) ServeHandshake(baseWriter http.ResponseWriter, req *http.Request) {
	authorization := req.Header.Get("Authorization")
	if authorization == "" {
		baseWriter.WriteHeader(http.StatusUnauthorized)
		return
	}
	// Save mobile key for use when client connects to stream
	srv.mobileKey = &authorization

	body, err := io.ReadAll(req.Body)
	if err != nil {
		baseWriter.WriteHeader(http.StatusBadRequest)
		return
	}

	// Save user for use when client connects to stream
	user := json.RawMessage(body)
	srv.user = &user

	json, err := json.Marshal(handshakePublicBundle{})
	if err != nil {
		baseWriter.WriteHeader(http.StatusInternalServerError)
		return
	}

	baseWriter.WriteHeader(http.StatusOK)

	_, _ = baseWriter.Write(json)
}
