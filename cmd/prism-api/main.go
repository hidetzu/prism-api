// Command prism-api is the HTTP entry point for prism-api.
package main

import (
	"fmt"
	"os"

	"github.com/hidetzu/prism-api/internal/app"
	"github.com/hidetzu/prism-api/internal/config"
	"github.com/hidetzu/prism-api/internal/logging"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}

	logger := logging.New(cfg)

	a := app.New(cfg, logger)
	if err := a.Run(); err != nil {
		logger.Error("server exited", "error", err)
		os.Exit(1)
	}
}
