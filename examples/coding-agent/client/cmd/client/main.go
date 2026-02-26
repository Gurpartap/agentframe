package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Gurpartap/agentframe/examples/coding-agent/client/internal/cmd"
)

func main() {
	if err := cmd.Execute(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
