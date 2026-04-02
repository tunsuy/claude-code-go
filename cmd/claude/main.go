package main

import (
	"os"

	"github.com/anthropics/claude-code-go/internal/bootstrap"
)

func main() {
	if err := bootstrap.Execute(); err != nil {
		os.Exit(1)
	}
}
