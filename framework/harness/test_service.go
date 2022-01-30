package harness

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework"
)

// TestServiceInfo is status information returned by the test service from the initial status query.
type TestServiceInfo struct {
	TestServiceInfoBase

	// FullData is the entire response received from the test service, which might contain additional
	// properties beyond TestServiceInfoBase.
	FullData []byte
}

// TestServiceInfoBase is the basic set of properties that all test services must provide.
type TestServiceInfoBase struct {
	// Name is the name of the project that the test service is testing, such as "go-server-sdk".
	Name string `json:"name"`

	// Capabilities is a list of strings representing optional features of the test service.
	Capabilities framework.Capabilities `json:"capabilities"`
}

// TestServiceEntity represents some kind of entity that we have asked the test service to create,
// which the test harness will interact with.
type TestServiceEntity struct {
	resourceURL string
	logger      framework.Logger
}

func queryTestServiceInfo(url string, timeout time.Duration, output io.Writer) (TestServiceInfo, error) {
	fmt.Fprintf(output, "Connecting to test service at %s", url)

	deadline := time.Now().Add(timeout)
	for {
		fmt.Fprintf(output, ".")
		resp, err := http.DefaultClient.Get(url)
		if err == nil {
			fmt.Fprintln(output)
			if resp.StatusCode != 200 {
				return TestServiceInfo{}, fmt.Errorf("test service returned status code %d", resp.StatusCode)
			}
			if resp.Body == nil {
				fmt.Fprintf(output, "Status query successful, but service provided no metadata\n")
				return TestServiceInfo{}, nil
			}
			respData, err := ioutil.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				return TestServiceInfo{}, err
			}
			fmt.Fprintf(output, "Status query returned metadata: %s\n", string(respData))
			var base TestServiceInfoBase
			if err := json.Unmarshal(respData, &base); err != nil {
				return TestServiceInfo{}, fmt.Errorf("malformed status response from test service: %s", string(respData))
			}
			return TestServiceInfo{TestServiceInfoBase: base, FullData: respData}, nil
		}
		if !time.Now().Before(deadline) {
			return TestServiceInfo{}, fmt.Errorf("timed out, result of last query was: %w", err)
		}
		time.Sleep(time.Millisecond * 100)
	}
}

// StopService tells the test service that it should exit.
func (h *TestHarness) StopService() error {
	req, _ := http.NewRequest("DELETE", h.testServiceBaseURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err == nil && resp.StatusCode >= 300 {
		return fmt.Errorf("service returned HTTP %d", resp.StatusCode)
	}
	// It's normal for the request to return an I/O error if the service immediately quit before sending a response
	return nil
}

// NewTestServiceEntity tells the test service to create a new instance of whatever kind of entity
// it manages, based on the parameters we provide. The test harness can interact with it via the
// returned TestServiceEntity. The entity is assumed to remain active inside the test service
// until we explicitly close it.
//
// The format of entityParams is defined by the test harness; this low-level method simply calls
// json.Marshal to convert whatever it is to JSON.
func (h *TestHarness) NewTestServiceEntity(
	entityParams interface{},
	description string,
	logger framework.Logger,
) (*TestServiceEntity, error) {
	if logger == nil {
		logger = framework.NullLogger()
	}

	data, err := json.Marshal(entityParams)
	if err != nil {
		return nil, err
	}

	logger.Printf("Creating test service entity (%s) with parameters: %s", description, string(data))
	_, headers, err := doRequest("POST", h.testServiceBaseURL, data)
	if err != nil {
		return nil, err
	}
	resourceURL := headers.Get("Location")
	if resourceURL == "" {
		return nil, errors.New("test service did not return a Location header with a resource URL")
	}
	if !strings.HasPrefix(resourceURL, "http:") {
		resourceURL = h.testServiceBaseURL + resourceURL
	}

	e := &TestServiceEntity{
		resourceURL: resourceURL,
		logger:      logger,
	}

	return e, nil
}

func doRequest(method, url string, body []byte) ([]byte, http.Header, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewBuffer(body)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, nil, err
	}
	if body != nil {
		req.Header.Add("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	var respBody []byte
	if resp.Body != nil {
		respBody, _ = ioutil.ReadAll(resp.Body)
		_ = resp.Body.Close()
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		message := ""
		if body != nil {
			message = " (" + string(body) + ")"
		}
		err = fmt.Errorf("test service returned error %d for %s %s%s", resp.StatusCode, method, url, message)
	}
	return respBody, resp.Header, err
}

// Close tells the test service to dispose of this entity.
func (e *TestServiceEntity) Close() error {
	e.logger.Printf("Closing %s", e.resourceURL)
	_, _, err := doRequest("DELETE", e.resourceURL, nil)
	if err != nil {
		e.logger.Printf("DELETE request to test service failed: %s", err)
	}
	return err
}

// SendCommand sends a command to the test service entity.
func (e *TestServiceEntity) SendCommand(
	command string,
	logger framework.Logger,
	responseOut interface{},
) error {
	return e.SendCommandWithParams(
		map[string]interface{}{"command": command},
		logger,
		responseOut,
	)
}

// SendCommandWithParams sends a command to the test service entity.
func (e *TestServiceEntity) SendCommandWithParams(
	allParams interface{},
	logger framework.Logger,
	responseOut interface{},
) error {
	if logger == nil {
		logger = e.logger
	}
	data, _ := json.Marshal(allParams)
	logger.Printf("Sending command: %s", string(data))
	body, _, err := doRequest("POST", e.resourceURL, data)
	if err != nil {
		return err
	}
	if responseOut != nil {
		if body == nil {
			return errors.New("expected a response body but got none")
		}
		if err = json.Unmarshal(body, responseOut); err != nil {
			return err
		}
		logger.Printf("Response: %s", string(body))
	}
	return nil
}
