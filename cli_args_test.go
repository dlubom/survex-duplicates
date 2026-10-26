package main

import "testing"

func TestOptionsPreserveInterspersedArguments(t *testing.T) {
	o, err := ParseOptions([]string{"input file.dump", "--out", "output file.csv", "--exclude-splays", "--batch-size=19"})
	if err != nil || o.Input != "input file.dump" || o.Out != "output file.csv" || o.Mode != "relative_percent" || o.Value != "5" || !o.ExcludeSplays || o.BatchSize != 19 {
		t.Fatalf("unexpected options: %+v, %v", o, err)
	}
}

func TestOptionsRejectInvalidSyntax(t *testing.T) {
	for _, args := range [][]string{nil, {"x"}, {"x", "--out", "y", "--epsilon", "1", "--relative-percent", "5"}, {"x", "--out", "y", "--batch-size", "0"}, {"x", "--out", "y", "--batch-size", "1.5"}, {"x", "--out", "y", "--input-format", "bad"}, {"x", "--out", "y", "--unknown"}, {"x", "z", "--out", "y"}} {
		if _, err := ParseOptions(args); err == nil {
			t.Fatalf("accepted %q", args)
		}
	}
}

func TestOptionsAbsoluteAndDashInput(t *testing.T) {
	o, err := ParseOptions([]string{"--epsilon", "5e-2", "--out=y", "--", "-input"})
	if err != nil || o.Input != "-input" || o.Mode != "absolute_m" || o.Value != "5e-2" {
		t.Fatalf("%+v %v", o, err)
	}
	o, err = ParseOptions([]string{"--help"})
	if err != nil || !o.Help {
		t.Fatalf("help: %+v %v", o, err)
	}
}

func TestOptionsRequireExplicitAssignmentForFlagLikePath(t *testing.T) {
	if _, err := ParseOptions([]string{"input", "--out", "-h"}); err == nil {
		t.Fatal("accepted -h as a separate output path")
	}
	o, err := ParseOptions([]string{"input", "--out=-h"})
	if err != nil || o.Out != "-h" {
		t.Fatalf("explicit path: %+v %v", o, err)
	}
}
