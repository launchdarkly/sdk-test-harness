package ldtest

import (
	"errors"
	"strings"
)

// Translates from the error format produced by testify/assert into a format that is
// friendlier to us. We want to put the message first, and we don't need to show intermediate
// stracktrace lines that are just part of our test infrastructure.
func reformatError(err error) error {
	if err == nil {
		return nil
	}
	traces, messages, ok := parseTestifyFailureMessage(err.Error())
	if !ok {
		return err
	}
	if len(messages) > 0 && strings.TrimSpace(messages[0]) == "Received unexpected error:" {
		messages = messages[1:]
		messages[0] = "Error: " + messages[0]
	}
	out := append([]string(nil), messages...)
	out = append(out, "  Error trace:")
	for _, line := range traces {
		if strings.Contains(line, "test_scope.go") {
			// This is a hack based on the fact that test_scope.go contains the T.Run method that
			// all test stacktraces should start at.
			break
		}
		out = append(out, "    "+line)
	}
	return errors.New(strings.Join(out, "\n"))
}

func parseTestifyFailureMessage(msg string) ([]string, []string, bool) {
	if !strings.Contains(msg, "Error Trace:") {
		return nil, nil, false
	}
	var traces []string
	var messages []string
	for _, line := range strings.Split(msg, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case len(messages) > 0:
			messages = append(messages, line)
		case len(traces) > 0:
			if strings.Contains(line, "Error:") {
				messages = append(messages, strings.TrimSpace(strings.TrimPrefix(line, "Error:")))
			} else {
				traces = append(traces, line)
			}
		default:
			if strings.Contains(line, "Error Trace:") {
				traces = append(traces, strings.TrimSpace(strings.TrimPrefix(line, "Error Trace:")))
			}
		}
	}
	return traces, messages, len(traces) > 0 && len(messages) > 0
}
