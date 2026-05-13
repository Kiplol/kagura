package main

import (
	"fmt"
	"os"

	"github.com/kip/kagura/internal/config"
	"github.com/kip/kagura/internal/tui"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load config: %v\n", err)
		cfg = config.Defaults()
	}

	if err := tui.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
