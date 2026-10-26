package main

import (
	"fmt"
	"os"

	"github.com/dlubom/survex-duplicates/internal/releasepublish"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: publish (configured through release environment variables)")
		os.Exit(2)
	}
	os.Exit(releasepublish.Run(releasepublish.Config{}, os.Stdout, os.Stderr))
}
