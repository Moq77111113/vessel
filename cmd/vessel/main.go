package main

import (
	"fmt"
	"os"

	"github.com/Moq77111113/vessel/internal/cli"
)

func main() {
	if err := cli.New().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "vessel:", err)
		os.Exit(1)
	}
}
