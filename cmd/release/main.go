package main

import (
	"github.com/dlubom/survex-duplicates/internal/releasepack"
	"os"
)

func main() { os.Exit(releasepack.Run(os.Args[1:], os.Stdout, os.Stderr)) }
