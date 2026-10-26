package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

const helpText = `Survex Duplicate Detector

Usage: survex-duplicates INPUT --out PATH [options]

Find geometric duplicate candidates at both endpoints, allowing reversed legs.
Default tolerance: 5% of the shorter leg, with no fixed centimetre cap.
After network adjustment, repeated readings may already have been averaged;
each match requires human review. DUPLICATE legs are excluded by default.

  --out PATH               Required CSV destination
  --relative-percent P     Positive percentage of the shorter leg (default 5)
  --epsilon M              Positive absolute endpoint tolerance in metres
  --include-duplicates     Include acknowledged DUPLICATE legs
  --exclude-splays         Exclude SPLAY legs
  --input-format FORMAT    auto, 3d or dump (default auto)
  --dump3d PATH             Survex executable (default dump3d in PATH)
  --batch-size B           Positive query batch size (default 1)
  --force                  Replace output only after successful completion
  --help, -h               Show this help
  --version                Show version and source revision

Tolerance options are mutually exclusive and accept exact decimal scientific notation.
Text dumps work without Survex. Binary .3d input requires dump3d --legs --show-dates.

Examples:
  survex-duplicates model.3d --out pairs.csv
  survex-duplicates model.dump --exclude-splays --out without-splays.csv
  survex-duplicates model.dump --epsilon 0.05 --out absolute.csv
`

func RunCLI(args []string, stdout, stderr io.Writer) int {
	o, err := ParseOptions(args)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 2
	}
	if o.Help {
		fmt.Fprint(stdout, helpText)
		return 0
	}
	if o.Version {
		fmt.Fprintf(stdout, "survex-duplicates %s (%s)\n", version, revision)
		return 0
	}
	tolerance := o.Tolerance
	if tolerance == nil {
		tolerance, err = NewTolerance(o.Mode, o.Value)
		if err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 2
		}
	}
	if err := run(o, tolerance, stderr); err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	return 0
}

func run(o Options, tolerance *Tolerance, stderr io.Writer) error {
	info, err := os.Stat(o.Input)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("input must be an existing regular file: %s", o.Input)
	}
	if err := ValidateDestination(o.Input, o.Out, o.Force); err != nil {
		return err
	}
	start := time.Now()
	model, err := ReadModel(o.Input, o.Format, o.Dump3D)
	if err != nil {
		return err
	}
	readSeconds := time.Since(start).Seconds()
	search := NewSearch(model, tolerance, o.ExcludeSplays, o.IncludeDuplicates)
	start = time.Now()
	if err := WriteReport(o.Out, model, search, tolerance, o.BatchSize, o.Force); err != nil {
		return err
	}
	unit := "percent"
	if tolerance.Mode == "absolute_m" {
		unit = "m"
	}
	return json.NewEncoder(stderr).Encode(map[string]any{
		"n_input": search.NInput, "n_excluded": search.NExcluded, "n": search.N,
		"C": search.Candidates, "K": search.Matches, "tolerance_mode": tolerance.Mode,
		"tolerance_value": CanonicalDecimal(tolerance.Value), "tolerance_unit": unit,
		"exclude_splays": o.ExcludeSplays, "include_duplicates": o.IncludeDuplicates,
		"batch_size": o.BatchSize, "input_format": model.InputFormat,
		"read_seconds": readSeconds, "build_seconds": search.BuildSeconds,
		"search_write_seconds": time.Since(start).Seconds(),
	})
}
