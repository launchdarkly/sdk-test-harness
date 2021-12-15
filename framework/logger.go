package framework

import (
	"fmt"
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

type CapturingLogger struct {
	output []CapturedMessage
	lock   sync.Mutex
}

func (l *CapturingLogger) Println(args ...interface{}) {
	l.lock.Lock()
	l.output = append(l.output, CapturedMessage{Time: time.Now(), Message: fmt.Sprintln(args...)})
	l.lock.Unlock()
}

func (l *CapturingLogger) Printf(message string, args ...interface{}) {
	l.lock.Lock()
	l.output = append(l.output, CapturedMessage{Time: time.Now(), Message: fmt.Sprintf(message, args...)})
	l.lock.Unlock()
}

func (l *CapturingLogger) Output() CapturedOutput {
	l.lock.Lock()
	ret := append([]CapturedMessage(nil), l.output...)
	l.lock.Unlock()
	return ret
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
