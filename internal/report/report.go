// Package report announces what a command is doing: one verb, right-aligned in a fixed
// column, and its subject. It imports nothing local.
package report

import (
	"fmt"
	"io"
)

// column is the width every verb is right-aligned into.
const column = 11

// Report is one line a command announces: a verb and what it applies to.
type Report interface {
	Line(verb, subject string)
}

// Writer prints lines to a stream, verb right-aligned in the column, then the subject.
type Writer struct {
	out io.Writer
}

// New returns a Report that writes to out.
func New(out io.Writer) *Writer { return &Writer{out: out} }

func (w *Writer) Line(verb, subject string) {
	fmt.Fprintf(w.out, "%*s %s\n", column, verb, subject)
}
