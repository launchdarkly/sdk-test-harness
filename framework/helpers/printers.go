package helpers

import (
	"fmt"
	"io"
)

// The refactoring that led to the creation of this file was to avoid the need
// to check for errors every time we print something. But, why are we printing
// things with Fprintf so much?
//
// It seems like we could be using the go logger instead to much greater
// effect.

func MustFprintln(w io.Writer, a ...any) {
	if _, err := fmt.Fprintln(w, a...); err != nil {
		panic(err)
	}
}

func MustFprintf(w io.Writer, format string, a ...any) {
	if _, err := fmt.Fprintf(w, format, a...); err != nil {
		panic(err)
	}
}
