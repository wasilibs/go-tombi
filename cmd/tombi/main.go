package main

import (
	"os"

	"github.com/wasilibs/go-tombi/internal/runner"
)

func main() {
	os.Exit(runner.Run("tombi", os.Args[1:], os.Stdin, os.Stdout, os.Stderr, "."))
}
