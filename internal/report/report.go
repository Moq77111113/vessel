package report

import (
	"fmt"
	"io"
)

const column = 11

type Report interface {
	Line(verb, subject string)
}

type Writer struct {
	out io.Writer
}

func New(out io.Writer) *Writer { return &Writer{out: out} }

func (w *Writer) Line(verb, subject string) {
	fmt.Fprintf(w.out, "%*s %s\n", column, verb, subject)
}
