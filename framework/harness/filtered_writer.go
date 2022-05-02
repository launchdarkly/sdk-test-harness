package harness

import (
	"io"
	"regexp"
)

type filteredWriter struct {
	writer       io.Writer
	excludeRegex []*regexp.Regexp
}

func newFilteredWriter(writer io.Writer, excludeRegex []*regexp.Regexp) *filteredWriter {
	return &filteredWriter{writer, excludeRegex}
}

func (f *filteredWriter) Write(data []byte) (int, error) {
	for _, r := range f.excludeRegex {
		if r.Match(data) {
			return len(data), nil
		}
	}
	return f.writer.Write(data)
}
