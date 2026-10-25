package main

import (
	"fmt"
	"os"
)

func main() {
	if err := Run(); err != nil {
		fmt.Fprintln(os.Stderr, "vessel:", err)
		os.Exit(Code(err))
	}
}
