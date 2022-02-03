package ldtest

import (
	"errors"
	"regexp"
	"runtime"
	"strings"
)

type ErrorWithStacktrace struct {
	Message    string
	Stacktrace []StacktraceInfo
}

type StacktraceInfo struct {
	FileName string
	Package  string
	Function string
	Line     int
}

func (e ErrorWithStacktrace) Error() string { return e.Message }

var errorTraceInMessageRegex = regexp.MustCompile(`^(?s:\s*Error Trace:.*\sError:\s*)`)

// transformError attaches a stacktrace to an error using our own stacktrace logic, and also
// strips out any stacktrace information that may have been added to the error message by the
// testify/assert or testify/require functions.
func transformError(err error, stacktrace []StacktraceInfo) error {
	message := err.Error()
	if strings.Contains(message, "Error Trace:") {
		message = strings.TrimSpace(errorTraceInMessageRegex.ReplaceAllLiteralString(message, ""))
	}
	if len(stacktrace) == 0 {
		return errors.New(message)
	}
	return ErrorWithStacktrace{Message: message, Stacktrace: stacktrace}
}

func currentPackageName() string {
	pc, _, _, ok := runtime.Caller(0)
	if !ok {
		return "?"
	}
	f := runtime.FuncForPC(pc)
	if f == nil {
		return "?"
	}
	packageName, _ := parsePackageAndFunctionName(f.Name())
	return packageName
}

func rootPackageName() string {
	p := currentPackageName()
	return strings.Join(strings.Split(p, "/")[0:3], "/")
}

func getStacktrace(includeLDTestCode bool, helperFns []string) []StacktraceInfo {
	callers := []StacktraceInfo{}
	currentPackage := currentPackageName()
StackLoop:
	for i := 1; ; i++ { // start at 1 because 0 would just be getStacktrace itself
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		f := runtime.FuncForPC(pc)
		if f == nil {
			break
		}
		parts := strings.Split(file, "/")
		file = parts[len(parts)-1]

		fullFunctionName := f.Name()
		packageName, functionName := parsePackageAndFunctionName(f.Name())

		if packageName == currentPackage && functionName == "Run" {
			break // ldtest.Run is always the root of the test run, no need to go further
		}
		if !includeLDTestCode && packageName == currentPackage {
			continue StackLoop
		}
		for _, helperFn := range helperFns {
			if helperFn == fullFunctionName {
				continue StackLoop // exclude this function from the stacktrace
			}
		}

		callers = append(callers, StacktraceInfo{FileName: file, Package: packageName, Function: functionName, Line: line})
	}
	return callers
}

func parsePackageAndFunctionName(fullName string) (string, string) {
	lastSlash := strings.LastIndex(fullName, "/")
	firstDotAfterSlash := strings.Index(fullName[lastSlash+1:], ".")
	packageName := fullName[0 : lastSlash+firstDotAfterSlash+1]
	functionName := fullName[len(packageName)+1:]
	return packageName, functionName
}

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
