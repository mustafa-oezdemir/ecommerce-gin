package main

import (
	"fmt"
	"os"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/application"
)

func main() {
	if err := application.Run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "application stopped: %v\n", err)
		os.Exit(1)
	}
}
