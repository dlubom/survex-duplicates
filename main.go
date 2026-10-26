package main

import "os"

func main() { os.Exit(RunCLI(os.Args[1:], os.Stdout, os.Stderr)) }
