package framework

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const timestampFormat = "2006-01-02 15:04:05.000"

type Logger interface {
	Println(args ...interface{})
	Printf(message string, args ...interface{})
}

type nullLogger struct{}

func (n nullLogger) Println(args ...interface{})                {}
func (n nullLogger) Printf(message string, args ...interface{}) {}

func NullLogger() Logger { return nullLogger{} }

type CapturedMessage struct {
	Time    time.Time
	Message string
}

type CapturedOutput []CapturedMessage

// CapturingLogger is used internally to record all output from a test scope. See comments on
// ldtest.(*T).DebugLogger() for the rules of logging in parent/child scopes.
type CapturingLogger struct {
	output   []CapturedMessage
	children []*CapturingLogger
	lock     sync.Mutex
}

func (l *CapturingLogger) Println(args ...interface{}) {
	m := strings.TrimRight(fmt.Sprintln(args...), "\r\n") // Sprintln appends a newline
	l.append(CapturedMessage{Time: time.Now(), Message: m})
}

func (l *CapturingLogger) Printf(message string, args ...interface{}) {
	l.append(CapturedMessage{Time: time.Now(), Message: fmt.Sprintf(message, args...)})
}

func (l *CapturingLogger) append(m CapturedMessage) {
	var children []*CapturingLogger
	l.lock.Lock()
	if len(l.children) == 0 {
		l.output = append(l.output, m)
	} else {
		children = append([]*CapturingLogger(nil), l.children...)
	}
	l.lock.Unlock()
	for _, c := range children {
		c.append(m)
	}
}

func (l *CapturingLogger) Output() CapturedOutput {
	l.lock.Lock()
	ret := append([]CapturedMessage(nil), l.output...)
	l.lock.Unlock()
	return ret
}

func (l *CapturingLogger) AddChildLogger(child *CapturingLogger) {
	l.lock.Lock()
	l.children = append(l.children, child)
	output := append([]CapturedMessage(nil), l.output...)
	l.lock.Unlock()
	child.lock.Lock()
	child.output = append(output, child.output...)
	child.lock.Unlock()
}

func (l *CapturingLogger) RemoveChildLogger(child *CapturingLogger) {
	l.lock.Lock()
	for i, c := range l.children {
		if c == child {
			l.children = append(l.children[0:i], l.children[i+1:]...)
			break
		}
	}
	l.lock.Unlock()
}

func (output CapturedOutput) ToString(prefix string) string {
	ret := ""
	for _, m := range output {
		if ret != "" {
			ret += "\n"
		}
		ret += fmt.Sprintf("%s[%s] %s",
			prefix,
			m.Time.Format(timestampFormat),
			m.Message,
		)
	}
	return ret
}

type prefixedLogger struct {
	base   Logger
	prefix string
}

func LoggerWithPrefix(baseLogger Logger, prefix string) Logger {
	return prefixedLogger{baseLogger, prefix}
}

func (p prefixedLogger) Println(args ...interface{}) {
	p.base.Println(append([]interface{}{p.prefix}, args...)...)
}

func (p prefixedLogger) Printf(message string, args ...interface{}) {
	p.base.Printf(p.prefix+message, args...)
}
