package main

import (
	"os"

	"github.com/mantle/mantle-ai/cli/internal/app"
)

func main() {
	runner := app.NewRunner()
	os.Exit(runner.Run(os.Args[1:]))
}
