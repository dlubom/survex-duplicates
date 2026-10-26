package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var cliNegativeNumber = regexp.MustCompile(`^-(?:[0-9]+|[0-9]*\.[0-9]+)$`)

type Options struct {
	Input, Out, Format, Dump3D, Mode, Value                string
	BatchSize                                              int
	ExcludeSplays, IncludeDuplicates, Force, Help, Version bool
	Tolerance                                              *Tolerance
}

func ParseOptions(args []string) (Options, error) {
	o := Options{Format: "auto", Dump3D: "dump3d", Mode: "relative_percent", Value: "5", BatchSize: 1}
	var positionals []string
	var toleranceOption string
	end := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if end || !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}
		if arg == "--" {
			end = true
			continue
		}
		name, value, assigned := strings.Cut(arg, "=")
		switch name {
		case "--help", "-h", "--version":
			if assigned {
				return o, fmt.Errorf("%s does not take a value", name)
			}
			o.Version = name == "--version"
			o.Help = !o.Version
			return o, nil
		case "--exclude-splays", "--include-duplicates", "--force":
			if assigned {
				return o, fmt.Errorf("%s does not take a value", name)
			}
			switch name {
			case "--exclude-splays":
				o.ExcludeSplays = true
			case "--include-duplicates":
				o.IncludeDuplicates = true
			case "--force":
				o.Force = true
			}
		case "--out", "--epsilon", "--relative-percent", "--batch-size", "--input-format", "--dump3d":
			if !assigned {
				i++
				if i >= len(args) || (strings.HasPrefix(args[i], "-") && args[i] != "-" && !cliNegativeNumber.MatchString(args[i])) {
					return o, fmt.Errorf("%s requires a value", name)
				}
				value = args[i]
			}
			switch name {
			case "--out":
				o.Out = value
			case "--dump3d":
				o.Dump3D = value
			case "--input-format":
				if value != "auto" && value != "3d" && value != "dump" {
					return o, fmt.Errorf("input format must be auto, 3d or dump")
				}
				o.Format = value
			case "--batch-size":
				b, err := strconv.Atoi(value)
				if err != nil || b <= 0 {
					return o, fmt.Errorf("batch size must be a positive integer")
				}
				o.BatchSize = b
			case "--epsilon", "--relative-percent":
				if toleranceOption != "" && toleranceOption != name {
					return o, fmt.Errorf("--epsilon and --relative-percent are mutually exclusive")
				}
				toleranceOption = name
				o.Value = value
				if name == "--epsilon" {
					o.Mode = "absolute_m"
				}
				var err error
				o.Tolerance, err = NewTolerance(o.Mode, value)
				if err != nil {
					return o, err
				}
			}
		default:
			return o, fmt.Errorf("unknown option %s", name)
		}
	}
	if len(positionals) != 1 || o.Out == "" {
		return o, fmt.Errorf("one INPUT and --out PATH are required")
	}
	o.Input = positionals[0]
	return o, nil
}
