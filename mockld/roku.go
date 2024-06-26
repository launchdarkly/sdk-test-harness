package mockld

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
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
