package ldtest

import (
	"errors"
	"fmt"
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

func (s StacktraceInfo) String() string {
	packageName := strings.TrimPrefix(s.Package, rootPackageName()+"/")
	return fmt.Sprintf("%s.%s (%s:%d)", packageName, s.Function, s.FileName, s.Line)
}

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
