package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cymonevo/go_template/internal/app"
)

func main() {
	application, err := app.New(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start application: %v\n", err)
		os.Exit(1)
	}

	if err := application.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "application exited with error: %v\n", err)
		os.Exit(1)
	}
}
