package main

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func reportFixture() *Model {
	return &Model{
		Legs: []Leg{
			{ID: 1, SourceLine: 7, A: Point{0, 0, 0}, B: Point{100, 100, 0}, Survey: " leading", Flags: []string{"SURFACE"}, Style: "NORMAL", Date: "2024.02.13"},
			{ID: 2, SourceLine: 8, A: Point{105, 105, 0}, B: Point{5, 5, 0}, Survey: "p],\"q[", Flags: []string{"DUPLICATE", "SPLAY"}},
		},
		Stations: map[Point]Station{
			{0, 0, 0}:     {Names: []string{"a", "Żółć\u2028<>", "back\\quote\"\t\x01"}, Anonymous: true, Status: "named"},
			{100, 100, 0}: {Anonymous: true, Status: "unnamed"},
		},
	}
}

func reportBytes(t *testing.T, model *Model, mode, value string, batch int) []byte {
	t.Helper()
	tolerance, err := NewTolerance(mode, value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "report.csv")
	if err := WriteReport(path, model, NewSearch(model, tolerance, false, true), tolerance, batch, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// This independently recorded Python report covers orientation, .17g, decimal
// square roots, leading-space CSV fields, Unicode and JSON escaping together.
func TestReportMatchesPythonCSVBytesAndBatchOrder(t *testing.T) {
	wantHeader := "leg_a,leg_b,survey_a,survey_b,source_line_a,source_line_b,distance_m,endpoint_1_error_m,endpoint_2_error_m,reversed,flags_a,flags_b,style_a,style_b,date_a,date_b,a_x1,a_y1,a_z1,a_x2,a_y2,a_z2,b_x1,b_y1,b_z1,b_x2,b_y2,b_z2,distance_cm,length_a_m,length_b_m,relative_error_percent,tolerance_mode,tolerance_value,tolerance_m,a1_station_names,a2_station_names,b1_station_names,b2_station_names,a1_station_status,a2_station_status,b1_station_status,b2_station_status,a1_has_anonymous_node,a2_has_anonymous_node,b1_has_anonymous_node,b2_has_anonymous_node\r\n"
	wantRow := "1,2, leading,\"p],\"\"q[\",7,8,0.070710678118654752,0.070710678118654752,0.070710678118654752,1,SURFACE,DUPLICATE|SPLAY,NORMAL,,2024.02.13,,0.00,0.00,0.00,1.00,1.00,0.00,1.05,1.05,0.00,0.05,0.05,0.00,7.0710678118654755,1.4142135623730951,1.4142135623730951,5,relative_percent,5,0.0707106781186547524400844362104849039285,\"[\"\"a\"\", \"\"Żółć\u2028<>\"\", \"\"back\\\\quote\\\"\"\\t\\u0001\"\"]\",[],[],[],named,unnamed,missing_node,missing_node,1,1,0,0\r\n"
	for _, batch := range []int{1, 2, 100} {
		got := reportBytes(t, reportFixture(), "relative_percent", "5", batch)
		if string(got) != wantHeader+wantRow {
			t.Fatalf("batch %d CSV mismatch\ngot  %q\nwant %q", batch, got, wantHeader+wantRow)
		}
	}
}

func TestReportDecimalToleranceMatchesPythonRounding(t *testing.T) {
	cases := []struct {
		value string
		end   Point
		want  string
	}{
		{"5", Point{}, "0"},
		{"5.00", Point{}, "0.00"},
		{"1e1000", Point{}, "0E+1000"},
		{"5.00", Point{100, 0, 0}, "0.05"},
		{"100.0", Point{100, 0, 0}, "1.0"},
		{"123.456", Point{500, 0, 0}, "6.1728"},
		{"5e-2", Point{1, 1, 0}, "0.00000707106781186547524400844362104849039285"},
		{"1e1000", Point{1, 1, 0}, "1.41421356237309504880168872420969807857E+996"},
		{"4.9999", Point{100, 100, 0}, "0.07070926390509237934503563452176069423042"},
		{"1.00000000000000000000000000000000000000000000000001", Point{1, 1, 1}, "0.00017320508075688772935274463415058723669428052538103979485638826682248683"},
	}
	for _, tc := range cases {
		t.Run(tc.value+"/"+tc.want, func(t *testing.T) {
			model := &Model{Legs: []Leg{{ID: 1, B: tc.end}, {ID: 2, B: tc.end}}}
			rows, err := csv.NewReader(bytes.NewReader(reportBytes(t, model, "relative_percent", tc.value, 1))).ReadAll()
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 2 || len(rows[1]) != 47 || rows[1][34] != tc.want {
				t.Fatalf("expected tolerance %q; rows %q", tc.want, rows)
			}
			if tc.end == (Point{}) && rows[1][31] != "" {
				t.Fatalf("point relative error must be empty: %q", rows[1][31])
			}
		})
	}
}

func TestReportExactInt32LengthsAndRelativeRatio(t *testing.T) {
	lo, hi := int64(-1<<31), int64(1<<31-1)
	model := &Model{Legs: []Leg{{ID: 1, A: Point{lo, lo, lo}, B: Point{hi, hi, hi}}, {ID: 2, A: Point{lo, lo, lo}, B: Point{hi, hi, hi}}}}
	rows, err := csv.NewReader(bytes.NewReader(reportBytes(t, model, "relative_percent", "1.2345678901234567890123456789", 1))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if rows[1][34] != "918407.5931894755904683096155844061526794133033814" {
		t.Fatalf("wrong exact length tolerance: %q", rows[1][34])
	}
	if rows[1][16] != "-21474836.48" || rows[1][19] != "21474836.47" {
		t.Fatalf("wrong coordinates: %q", rows[1][16:22])
	}
}

func TestReportPreservesAbsoluteDecimalRepresentation(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{"+5", "5"}, {"5.00", "5.00"}, {"5e-2", "0.05"}, {"1e1000", "1E+1000"}, {"1e-7", "1E-7"}, {"1e-6", "0.000001"}, {"123e2", "1.23E+4"}, {"00100.000", "100.000"}, {"100e-2", "1.00"}} {
		model := &Model{Legs: []Leg{{ID: 1}, {ID: 2}}}
		rows, err := csv.NewReader(bytes.NewReader(reportBytes(t, model, "absolute_m", tc.input, 1))).ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		if rows[1][33] != tc.want || rows[1][34] != tc.want {
			t.Fatalf("decimal %q: got %q / %q", tc.input, rows[1][33], rows[1][34])
		}
	}
}

func TestReportPublicationPreservesExistingFilesAndCleansTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.csv")
	if err := os.WriteFile(path, []byte("old report"), 0600); err != nil {
		t.Fatal(err)
	}
	model := reportFixture()
	tolerance, err := NewTolerance("relative_percent", "5")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		batch int
		force bool
	}{{1, false}, {0, true}} {
		err := WriteReport(path, model, NewSearch(model, tolerance, false, true), tolerance, tc.batch, tc.force)
		if err == nil {
			t.Fatalf("expected publication failure for %+v", tc)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "old report" {
			t.Fatalf("old output damaged: %q, %v", got, err)
		}
		files, err := os.ReadDir(dir)
		if err != nil || len(files) != 1 {
			t.Fatalf("temporary files remain: %v, %v", files, err)
		}
	}
	if err := WriteReport(path, model, NewSearch(model, tolerance, false, true), tolerance, 1, true); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !strings.HasPrefix(string(got), "leg_a,") {
		t.Fatalf("force did not publish: %q, %v", got, err)
	}
}

func TestValidateDestinationRejectsInputAliasesAndExistingOutputs(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.dump")
	if err := os.WriteFile(source, []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	symlink, hardlink := filepath.Join(dir, "symlink"), filepath.Join(dir, "hardlink")
	if err := os.Symlink(source, symlink); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(source, hardlink); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{source, symlink, hardlink} {
		if err := ValidateDestination(source, path, true); err == nil {
			t.Fatalf("accepted input alias %q", path)
		}
	}
	output := filepath.Join(dir, "output.csv")
	if err := os.WriteFile(output, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDestination(source, output, false); err == nil {
		t.Fatal("accepted existing report without force")
	}
	if err := ValidateDestination(source, output, true); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(dir, "broken")
	if err := os.Symlink(filepath.Join(dir, "missing"), broken); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDestination(source, broken, false); err == nil {
		t.Fatal("accepted existing dangling symlink without force")
	}
	if err := ValidateDestination(source, filepath.Join(dir, "new.csv"), false); err != nil {
		t.Fatal(err)
	}
}

func TestReportEmptyInputWritesACompleteHeader(t *testing.T) {
	data := reportBytes(t, &Model{}, "relative_percent", "5", 1)
	rows, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0]) != 47 || !bytes.HasSuffix(data, []byte("\r\n")) {
		t.Fatalf("incomplete empty report: %q", data)
	}
}

func TestReportFailedPublicationPreservesDestinationAndRemovesTemporary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing-directory")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(path, "keep")
	if err := os.WriteFile(sentinel, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	model := reportFixture()
	tolerance, err := NewTolerance("relative_percent", "5")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteReport(path, model, NewSearch(model, tolerance, false, true), tolerance, 1, true); err == nil {
		t.Fatal("replacing a nonempty directory unexpectedly succeeded")
	}
	data, err := os.ReadFile(sentinel)
	if err != nil || string(data) != "original" {
		t.Fatalf("failed publication damaged existing content: %q, %v", data, err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 || files[0].Name() != "existing-directory" {
		t.Fatalf("temporary report remains after publication failure: %v, %v", files, err)
	}
}
