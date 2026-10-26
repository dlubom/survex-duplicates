package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

const readerTestHeader = "TITLE \"Reader fixture\"\nDATE \"@1\"\nDATE_NUMERIC 1\nVERSION 8\nSEPARATOR '.'\n--\n"
const readerHelperMode = "SURVEX_READER_TEST_HELPER_MODE"

func readerFullDump(body string) string { return readerTestHeader + body + "\nSTOP\n" }

// Reuse the compiled test binary as dump3d on every platform. TestMain runs
// before testing parses flags, so the child's --legs/--show-dates remain intact.
func TestMain(m *testing.M) {
	if mode := os.Getenv(readerHelperMode); mode != "" {
		os.Exit(runReaderProcessHelper(mode))
	}
	os.Exit(m.Run())
}

func runReaderProcessHelper(mode string) int {
	if len(os.Args) != 4 || os.Args[1] != "--legs" || os.Args[2] != "--show-dates" || !filepath.IsAbs(os.Args[3]) {
		fmt.Fprintln(os.Stderr, "unexpected dump3d arguments")
		return 24
	}
	if _, err := os.Stat(os.Args[3]); err != nil {
		fmt.Fprintln(os.Stderr, "input path was not preserved:", err)
		return 25
	}
	switch mode {
	case "exit-empty", "exit-valid":
		fmt.Fprint(os.Stderr, "unrecognized option --legs")
		if mode == "exit-valid" {
			fmt.Fprint(os.Stdout, readerFullDump(""))
		}
		return 9
	case "locale", "locale-case":
		if _, exists := os.LookupEnv("LC_ALL"); exists {
			return 21
		}
		if os.Getenv("LC_NUMERIC") != "C" {
			return 22
		}
		if os.Getenv("LC_TIME") != "pl_PL.UTF-8" {
			return 23
		}
		if mode == "locale-case" && runtime.GOOS != "windows" && (os.Getenv("lc_all") != "lowercase-all" || os.Getenv("lc_numeric") != "lowercase-numeric") {
			fmt.Fprintln(os.Stderr, "Unix lowercase environment keys were changed")
			return 29
		}
		if _, err := io.WriteString(os.Stderr, strings.Repeat("x", 2000000)); err != nil {
			return 26
		}
		fmt.Fprint(os.Stdout, readerFullDump("LEG 0 0 0 1 0 0 []"))
	case "malformed", "invalid-utf8":
		prefix := "invalid\n"
		if mode == "invalid-utf8" {
			prefix = "\xff\n"
		}
		fmt.Fprint(os.Stdout, prefix)
		if _, err := io.WriteString(os.Stdout, strings.Repeat("\x00", 2000000)); err != nil {
			return 27
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown reader helper mode")
		return 28
	}
	return 0
}

func readerProcessHelper(t *testing.T, mode string) string {
	t.Helper()
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(readerHelperMode, mode)
	return program
}

func readerFixture(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join("tests", "fixtures")
	provenance := readVerifiedKataster(t, filepath.Join(dir, "provenance.json"),
		"282ad9665dc31ce37f013369cfb262c522021c5f41dfd220c1a513f9951fd11a")
	var saved struct {
		SHA256 map[string]string `json:"sha256"`
	}
	if err := json.Unmarshal(provenance, &saved); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != saved.SHA256[name] {
		t.Fatalf("fixture hash mismatch: %s", name)
	}
	return path
}

func TestReaderRealLegsAndLines(t *testing.T) {
	legs, err := ReadModel(readerFixture(t, "survey.legs.dump"), "auto", "")
	if err != nil {
		t.Fatal(err)
	}
	wantStarts := []Point{{0, 0, 0}, {100, 0, 0}, {100, 101, 0}, {0, 0, 0}, {100, 0, 0}, {0, 0, 0}}
	if len(legs.Legs) != len(wantStarts) {
		t.Fatalf("legs: %d", len(legs.Legs))
	}
	for i, leg := range legs.Legs {
		if leg.ID != i+1 || leg.A != wantStarts[i] {
			t.Fatalf("leg %d: %+v", i, leg)
		}
	}
	want := Leg{ID: 4, SourceLine: 10, A: Point{0, 0, 0}, B: Point{100, 0, 0}, Survey: `p],"q[`, Flags: []string{"DUPLICATE"}, Style: "CARTESIAN", Date: "2024.02.13"}
	if !reflect.DeepEqual(legs.Legs[3], want) {
		t.Fatalf("leg: %+v, want %+v", legs.Legs[3], want)
	}
	lines, err := ReadModel(readerFixture(t, "survey.lines.dump"), "dump", "")
	if err != nil {
		t.Fatal(err)
	}
	for i := range lines.Legs {
		lines.Legs[i].SourceLine = legs.Legs[i].SourceLine
	}
	if !reflect.DeepEqual(legs, lines) {
		t.Fatalf("LEG and MOVE/LINE models differ")
	}
}

func TestReaderNodesAndExactCoordinates(t *testing.T) {
	body := "NODE 0 0 0 [Żółta:alias]\nMOVE 0 0 0\nMOVE 1 0 0\nNODE 0 0 0 [a],\"b[] UNDERGROUND\nNODE 0 0 0 [] ANON\nNODE 0 0 0 [Żółta:alias]\nLINE 0 0 0 []\nNODE 2 0 0 []"
	model, err := ParseDump(strings.NewReader(readerFullDump(body)))
	if err != nil {
		t.Fatal(err)
	}
	want := Station{Names: []string{`a],"b[`, "Żółta:alias"}, Anonymous: true, Status: "named"}
	if !reflect.DeepEqual(model.Stations[Point{}], want) {
		t.Fatalf("station: %+v", model.Stations[Point{}])
	}
	if model.Stations[Point{200, 0, 0}].Status != "unnamed" {
		t.Fatal("missing unnamed station")
	}
	if _, found := model.Stations[Point{100, 0, 0}]; found {
		t.Fatal("invented NODE")
	}
	if model.Legs[0].A != (Point{100, 0, 0}) {
		t.Fatal("NODE changed current position")
	}
	model, err = ParseDump(strings.NewReader(readerFullDump("LEG -21474836.48 +0.000 1e-2 21474836.47 .0 -1E-2 [] SURFACE SPLAY SURFACE")))
	if err != nil {
		t.Fatal(err)
	}
	if model.Legs[0].A != (Point{-2147483648, 0, 1}) || model.Legs[0].B != (Point{2147483647, 0, -1}) {
		t.Fatalf("exact coordinates: %+v", model.Legs[0])
	}
	if !reflect.DeepEqual(model.Legs[0].Flags, []string{"SPLAY", "SURFACE"}) {
		t.Fatalf("flags: %v", model.Legs[0].Flags)
	}
}

func TestReaderLegacyAndClosureMetadata(t *testing.T) {
	legacy, err := ReadModel(readerFixture(t, "legacy.legs.dump"), "auto", "")
	if err != nil {
		t.Fatal(err)
	}
	modern, err := ReadModel(readerFixture(t, "closure.legs.dump"), "auto", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy.Legs) != 3 || len(modern.Legs) != 3 {
		t.Fatal("unexpected leg count")
	}
	for i := range legacy.Legs {
		a, b := legacy.Legs[i], modern.Legs[i]
		if a.A != b.A || a.B != b.B || a.Survey != "" || a.Style != "" || a.Date != "" || b.Style != "NORMAL" {
			t.Fatalf("leg %d: %+v, %+v", i, a, b)
		}
	}
	_, err = ReadModel(readerFixture(t, "extended.legs.dump"), "auto", "")
	if err == nil || !strings.Contains(err.Error(), "ordinary spatial model") {
		t.Fatalf("extended: %v", err)
	}
}

func TestReaderRejectsMalformedRecords(t *testing.T) {
	for _, body := range []string{
		"CODE_0x42 1", "<<< REWIND <<<", "LINE 1 2 3 []", "LEG 0 0 0 1 0 []", "LEG 0 0 bad 1 0 0 []",
		"LEG 0 0 0 1 0 0 []\nMOVE 0 0 0\nLINE 1 0 0 []", "LEG 0.001 0 0 1 0 0 []", "LEG 21474836.48 0 0 1 0 0 []",
		"LEG -21474836.49 0 0 1 0 0 []", "LEG 1e-3 0 0 1 0 0 []", "LEG 1e100 0 0 1 0 0 []", "LEG 0x10 0 0 1 0 0 []", "LEG NaN 0 0 1 0 0 []",
		"LEG 0 0 0 1 0 0 [] 2024.02.30", "LEG 0 0 0 1 0 0 [] 2024.03.01-2024.02.29", "LEG 0 0 0 1 0 0 [] 0000.01.01",
		"LEG 0 0 0 1 0 0 [] UNKNOWN", "LEG 0 0 0 1 0 0 [] STYLE=lower", "LEG 0 0 0 1 0 0 [x]SPLAY",
		"NODE 0 0 0 [x] UNKNOWN", "NODE 0 0 0 x]", "NODE 0 0 0 [x", "MOVE 0 0 0 extra",
		"XSECT broken", "XSECT -1.01 0 0 0 [x]", "XSECT 0 0 0 0 [x] 2024.02.30", "XSECT_END broken", "ERROR_INFO broken",
		"ERROR_INFO #legs 3, len -1m, E 0 H 0 V 0", "STOP extra", "EXTENDED ELEVATION",
	} {
		t.Run(body, func(t *testing.T) {
			_, err := ParseDump(strings.NewReader(readerFullDump(body)))
			if err == nil || !strings.Contains(err.Error(), "line ") {
				t.Fatalf("expected line diagnostic, got %v", err)
			}
		})
	}
}

func TestReaderRejectsMalformedStreams(t *testing.T) {
	for _, input := range []string{"", "STOP\n", "TITLE broken\n--\nSTOP\n", readerTestHeader,
		readerFullDump("") + "LEG 0 0 0 1 0 0 []\n", readerFullDump("") + readerFullDump(""),
		strings.Replace(readerFullDump(""), "VERSION 8", "VERSION 9", 1),
		strings.Replace(readerFullDump(""), "DATE_NUMERIC 1\nVERSION 8", "VERSION 8\nDATE_NUMERIC 1", 1),
		strings.Replace(readerFullDump(""), "SEPARATOR '.'", "SEPARATOR '..'", 1),
		readerFullDump("NODE 0 0 0 [\xff]"),
	} {
		_, err := ParseDump(strings.NewReader(input))
		if err == nil || !strings.Contains(err.Error(), "line ") {
			t.Errorf("expected line diagnostic for %q, got %v", input, err)
		}
	}
}

func TestReaderValidNonLegRecordsAndLongNames(t *testing.T) {
	name := strings.Repeat("x", 100000) + `],"q[`
	body := "XSECT -1 0 1e-2 1 [a],\"q[] 2024.02.29\nXSECT_END\nERROR_INFO #legs 0, len 0m, E 1e-3 H +.0 V 1.\nNODE 0 0 0 [" + name + "]"
	model, err := ParseDump(strings.NewReader(readerFullDump(body)))
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Legs) != 0 || !reflect.DeepEqual(model.Stations[Point{}].Names, []string{name}) {
		t.Fatal("non-leg records or long name lost")
	}
}

func TestReaderNonLegDecimalExponentHasNoMagnitudeLimit(t *testing.T) {
	for _, body := range []string{
		"XSECT 1e1000001 0 0 0 []",
		"XSECT -1e-1000001 -0e1000001 0 0 []",
		"XSECT -1e-999999999999999999999999999999 1e999999999999999999999999999999 0 0 []",
		"XSECT -100000000000000000000000000000000e-32 0 0 0 []",
		"ERROR_INFO #legs 1, len 1e-1000001m, E 1e1000001 H 0 V -0e999999999999999999999999999999",
	} {
		model, err := ParseDump(strings.NewReader(readerFullDump(body)))
		if err != nil || len(model.Legs) != 0 {
			t.Fatalf("valid non-leg record %q: %v", body, err)
		}
	}
	for _, body := range []string{
		"XSECT -1e1000001 0 0 0 []",
		"XSECT -1e999999999999999999999999999999 0 0 0 []",
		"XSECT -100000000000000000000000000000001e-32 0 0 0 []",
		"ERROR_INFO #legs 1, len -1e-1000001m, E 0 H 0 V 0",
	} {
		if _, err := ParseDump(strings.NewReader(readerFullDump(body))); err == nil {
			t.Fatalf("accepted out-of-range non-leg record %q", body)
		}
	}
}

func TestReaderBinaryInputAndFormatDetection(t *testing.T) {
	if _, err := exec.LookPath("dump3d"); err != nil {
		t.Skip("Survex unavailable")
	}
	source := readerFixture(t, "survey.3d")
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "model ze spacją Żółć.not3d")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	actual, err := ReadModel(path, "auto", "dump3d")
	if err != nil {
		t.Fatal(err)
	}
	want, err := ReadModel(readerFixture(t, "survey.legs.dump"), "auto", "")
	if err != nil {
		t.Fatal(err)
	}
	if actual.InputFormat != "3d" {
		t.Fatal(actual.InputFormat)
	}
	actual.InputFormat = "dump"
	if !reflect.DeepEqual(actual, want) {
		t.Fatal("binary and recorded dump differ")
	}
}

func TestReaderChildProcessContract(t *testing.T) {
	data, err := os.ReadFile(readerFixture(t, "survey.3d"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "model ze spacją Żółć.3d")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"exit-empty", "exit-valid"} {
		t.Run(mode, func(t *testing.T) {
			program := readerProcessHelper(t, mode)
			_, err := ReadModel(path, "3d", program)
			if err == nil || !strings.Contains(err.Error(), "code 9") || !strings.Contains(err.Error(), "--legs") {
				t.Fatalf("process error: %v", err)
			}
		})
	}
	program := readerProcessHelper(t, "locale")
	t.Setenv("LC_ALL", "pl_PL.UTF-8")
	t.Setenv("LC_TIME", "pl_PL.UTF-8")
	model, err := ReadModel(path, "3d", program)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Legs) != 1 {
		t.Fatal("missing child output")
	}
	_, err = ReadModel(path, "3d", filepath.Join(t.TempDir(), "missing"))
	if err == nil || !strings.Contains(err.Error(), "dump3d not found") {
		t.Fatal(err)
	}
}

func TestReaderLocaleEnvironmentUsesPlatformCaseRules(t *testing.T) {
	program := readerProcessHelper(t, "locale-case")
	t.Setenv("LC_ALL", "original-all")
	t.Setenv("LC_NUMERIC", "original-numeric")
	if runtime.GOOS == "windows" {
		// Remove inherited uppercase spellings before setting lowercase names,
		// since Windows treats both spellings as one environment variable.
		for _, key := range []string{"LC_ALL", "LC_NUMERIC"} {
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Setenv("lc_all", "lowercase-all")
	t.Setenv("lc_numeric", "lowercase-numeric")
	t.Setenv("LC_TIME", "pl_PL.UTF-8")
	model, err := ReadModel(readerFixture(t, "survey.3d"), "3d", program)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Legs) != 1 {
		t.Fatal("missing child output")
	}
}

func TestReaderCoordinateAgreementWithRationalOracle(t *testing.T) {
	values := []string{"1.", "+.01", "-.0", "00000000000000000000000000000000.01000000", "2.147483647e7", "-2.147483648e7", "2147483647e-2", "2147483648e-2", "00001.000e+0000", "1e-100", "0e100000000000000000000000000000000000000"}
	for _, coefficient := range []int64{-2147483648001, -2147483648000, -1000000, -1, 0, 1, 10, 1010, 2147483647000, 2147483647001} {
		for exponent := -16; exponent <= 16; exponent++ {
			values = append(values, fmt.Sprintf("%de%d", coefficient, exponent))
		}
	}
	for _, value := range values {
		model, err := ParseDump(strings.NewReader(readerFullDump("LEG " + value + " 0 0 0 0 0 []")))
		if strings.HasPrefix(value, "0e100000000000000000000000000000000000000") {
			if err != nil || model.Legs[0].A[0] != 0 {
				t.Fatalf("zero with large exponent: %v", err)
			}
			continue
		}
		n, ok := new(big.Rat).SetString(value)
		if !ok {
			t.Fatalf("invalid oracle input %q", value)
		}
		n.Mul(n, big.NewRat(100, 1))
		valid := n.IsInt() && n.Num().IsInt64() && n.Num().Int64() >= -2147483648 && n.Num().Int64() <= 2147483647
		if valid {
			if err != nil || model.Legs[0].A[0] != n.Num().Int64() {
				t.Errorf("%s: want %v, model=%+v error=%v", value, n, model, err)
			}
		} else if err == nil {
			t.Errorf("accepted off-grid/out-of-range %s", value)
		}
	}
}

type readerFailAfterData struct{ data string }

func (r *readerFailAfterData) Read(p []byte) (int, error) {
	if r.data == "" {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func TestReaderStreamErrorsAndUniversalNewlines(t *testing.T) {
	want, err := ParseDump(strings.NewReader(readerFullDump("LEG 0 0 0 1 0 0 []")))
	if err != nil {
		t.Fatal(err)
	}
	for _, newline := range []string{"\r", "\r\n"} {
		actual, err := ParseDump(strings.NewReader(strings.ReplaceAll(readerFullDump("LEG 0 0 0 1 0 0 []"), "\n", newline)))
		if err != nil || !reflect.DeepEqual(actual, want) {
			t.Fatalf("newline %q: %+v, %v", newline, actual, err)
		}
	}
	_, err = ParseDump(&readerFailAfterData{data: readerFullDump("")})
	if err == nil || !strings.Contains(err.Error(), "reading dump") {
		t.Fatalf("lost read error: %v", err)
	}
}

func TestReaderBinaryMalformedStreamDrainsAndRejectsUTF8(t *testing.T) {
	for _, mode := range []string{"malformed", "invalid-utf8"} {
		program := readerProcessHelper(t, mode)
		_, err := ReadModel(readerFixture(t, "survey.3d"), "3d", program)
		if err == nil || !strings.Contains(err.Error(), "line 1:") {
			t.Fatalf("parse error: %v", err)
		}
	}
	_, err := ReadModel(readerFixture(t, "survey.3d"), "invalid", "")
	if err == nil || !strings.Contains(err.Error(), "input_format") {
		t.Fatalf("format error: %v", err)
	}
	if _, err = exec.LookPath("dump3d"); err == nil {
		_, err = ReadModel(readerFixture(t, "extended.3d"), "3d", "dump3d")
		if err == nil || !strings.Contains(err.Error(), "ordinary spatial model") {
			t.Fatalf("extended binary: %v", err)
		}
	}
}
