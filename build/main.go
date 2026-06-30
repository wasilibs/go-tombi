package main

import (
	"github.com/goyek/x/boot"
	"github.com/wasilibs/tools/tasks"
)

func main() {
	tasks.Define(tasks.Params{
		LibraryName: "tombi",
		LibraryRepo: "tombi-toml/tombi",
		GoReleaser:  true,
	})
	boot.Main()
}
