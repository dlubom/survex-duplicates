package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const readerSignature = "Survex 3D Image File\n"
const readerDecimalSyntax = `[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?`

var (
	readerNumberPattern    = regexp.MustCompile(`^` + readerDecimalSyntax + `$`)
	readerDatePattern      = regexp.MustCompile(`^[0-9]{4}\.[0-9]{2}\.[0-9]{2}(?:-[0-9]{4}\.[0-9]{2}\.[0-9]{2})?$`)
	readerStylePattern     = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	readerIntegerPattern   = regexp.MustCompile(`^-?[0-9]+$`)
	readerErrorInfoPattern = regexp.MustCompile(`^ERROR_INFO #legs [0-9]+, len (` + readerDecimalSyntax + `)m, E (` + readerDecimalSyntax + `) H (` + readerDecimalSyntax + `) V (` + readerDecimalSyntax + `)$`)
)

// Python's whitespace split also includes the four ASCII information separators.
func readerSpace(r rune) bool        { return unicode.IsSpace(r) || r >= '\x1c' && r <= '\x1f' }
func readerFields(s string) []string { return strings.FieldsFunc(s, readerSpace) }

// Preserve the remainder verbatim: names may include spaces and unescaped brackets.
func readerPrefix(s string, count int) ([]string, string, error) {
	fields := make([]string, 0, count)
	for range count {
		s = strings.TrimLeftFunc(s, readerSpace)
		if s == "" {
			return nil, "", errors.New("missing record fields")
		}
		i := strings.IndexFunc(s, readerSpace)
		if i < 0 {
			fields = append(fields, s)
			s = ""
		} else {
			fields = append(fields, s[:i])
			s = s[i:]
		}
	}
	return fields, strings.TrimLeftFunc(s, readerSpace), nil
}

// Non-leg values only need comparisons with zero or -1. Compare their exact
// decimal sign and magnitude without allocating a power of an input exponent.
func readerNonLegNumber(s string, allowNegativeUnit bool) (bool, error) {
	if !readerNumberPattern.MatchString(s) {
		return false, fmt.Errorf("invalid number %q", s)
	}
	negative := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "+"), "-")
	exponent := "0"
	if split := strings.IndexAny(s, "eE"); split >= 0 {
		exponent = s[split+1:]
		s = s[:split]
	}
	places := 0
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		places = len(s) - dot - 1
		s = s[:dot] + s[dot+1:]
	}
	digits := strings.TrimLeft(s, "0")
	if digits == "" || !negative {
		return true, nil
	}
	if !allowNegativeUnit {
		return false, nil
	}
	// Compare exponent with the value making the leading significant digit
	// occupy the units position. This limit is bounded by the input length.
	limit := int64(places) - int64(len(digits)) + 1
	value, err := strconv.ParseInt(exponent, 10, 64)
	if err != nil {
		// Valid syntax leaves only int64 overflow: its sign determines which
		// side of any representable input-length bound the exponent occupies.
		return strings.HasPrefix(exponent, "-"), nil
	}
	if value != limit {
		return value < limit, nil
	}
	return digits[0] == '1' && strings.Trim(digits[1:], "0") == "", nil
}

// Decimal coordinates use integer accumulation. Scientific notation takes the
// exact decimal-normalization path; neither path rounds through floating point.
func readerCoordinate(s string) (int64, error) {
	if strings.ContainsAny(s, "eE") {
		return readerExponentCoordinate(s)
	}
	original := s
	negative := false
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		negative = s[0] == '-'
		s = s[1:]
	}
	var whole, fraction int64
	digits, places := 0, 0
	dot, overflow, offGrid := false, false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '.' && !dot {
			dot = true
			continue
		}
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid number %q", original)
		}
		digits++
		if !dot {
			if whole > 21474836 {
				overflow = true
			} else {
				whole = whole*10 + int64(c-'0')
			}
		} else {
			places++
			if places <= 2 {
				fraction = fraction*10 + int64(c-'0')
			} else if c != '0' {
				offGrid = true
			}
		}
	}
	if digits == 0 {
		return 0, fmt.Errorf("invalid number %q", original)
	}
	if places == 1 {
		fraction *= 10
	}
	value := whole*100 + fraction
	if negative {
		value = -value
	}
	if overflow || offGrid || value < -2147483648 || value > 2147483647 {
		return 0, errors.New("coordinates must be on the centimetre grid and within int32 range")
	}
	return value, nil
}

func readerExponentCoordinate(s string) (int64, error) {
	if !readerNumberPattern.MatchString(s) {
		return 0, fmt.Errorf("invalid number %q", s)
	}
	i := strings.IndexAny(s, "eE")
	mantissa, exponent := s[:i], s[i+1:]
	negative := mantissa[0] == '-'
	if mantissa[0] == '-' || mantissa[0] == '+' {
		mantissa = mantissa[1:]
	}
	places := 0
	if dot := strings.IndexByte(mantissa, '.'); dot >= 0 {
		places = len(mantissa) - dot - 1
		mantissa = mantissa[:dot] + mantissa[dot+1:]
	}
	mantissa = strings.TrimLeft(mantissa, "0")
	if mantissa == "" {
		return 0, nil
	}
	e, err := strconv.ParseInt(exponent, 10, 64)
	invalid := errors.New("coordinates must be on the centimetre grid and within int32 range")
	if err != nil {
		return 0, invalid
	}
	// Exponents outside these bounds cannot place a nonzero value in int32 cm.
	// Compare before addition to avoid overflowing on a valid int64 exponent.
	if e > int64(places)+10 || e < -int64(len(mantissa))-2 {
		return 0, invalid
	}
	scale := e + 2 - int64(places)
	for scale < 0 && strings.HasSuffix(mantissa, "0") {
		mantissa = mantissa[:len(mantissa)-1]
		scale++
	}
	if scale < 0 || int64(len(mantissa))+scale > 10 {
		return 0, invalid
	}
	value, err := strconv.ParseInt(mantissa, 10, 64)
	if err != nil {
		return 0, invalid
	}
	for ; scale > 0; scale-- {
		value *= 10
	}
	if negative {
		value = -value
	}
	if value < -2147483648 || value > 2147483647 {
		return 0, invalid
	}
	return value, nil
}

func readerPoint(fields []string) (Point, error) {
	var point Point
	if len(fields) != 3 {
		return point, errors.New("coordinates must contain three values")
	}
	for i, s := range fields {
		value, err := readerCoordinate(s)
		if err != nil {
			return point, err
		}
		point[i] = value
	}
	return point, nil
}

func readerNamed(s string) (string, []string, error) {
	i := strings.LastIndexByte(s, ']')
	if !strings.HasPrefix(s, "[") || i < 1 {
		return "", nil, errors.New("missing [name] field")
	}
	suffix := s[i+1:]
	if suffix != "" {
		first, _ := utf8.DecodeRuneInString(suffix)
		if !readerSpace(first) {
			return "", nil, errors.New("missing [name] field")
		}
	}
	return s[1:i], readerFields(suffix), nil
}

func readerSurveyDate(s string) error {
	if !readerDatePattern.MatchString(s) {
		return fmt.Errorf("invalid survey date %q", s)
	}
	parts := strings.Split(s, "-")
	var previous time.Time
	for _, part := range parts {
		value, err := time.Parse("2006.01.02", part)
		if err != nil || value.Year() < 1 {
			return fmt.Errorf("invalid survey date %q", s)
		}
		if !previous.IsZero() && value.Before(previous) {
			return errors.New("survey date range is reversed")
		}
		previous = value
	}
	return nil
}

func readerLegSuffix(suffix []string) ([]string, string, string, error) {
	style, date := "", ""
	if len(suffix) > 0 && strings.HasPrefix(suffix[0], "STYLE=") {
		style = suffix[0][6:]
		suffix = suffix[1:]
		if !readerStylePattern.MatchString(style) {
			return nil, "", "", errors.New("invalid STYLE")
		}
	}
	if len(suffix) > 0 && suffix[len(suffix)-1][0] >= '0' && suffix[len(suffix)-1][0] <= '9' {
		date = suffix[len(suffix)-1]
		suffix = suffix[:len(suffix)-1]
		if err := readerSurveyDate(date); err != nil {
			return nil, "", "", err
		}
	}
	var flags []string
	for _, flag := range suffix {
		switch flag {
		case "SURFACE", "DUPLICATE", "SPLAY":
		default:
			return nil, "", "", fmt.Errorf("unknown leg metadata: %s", strings.Join(suffix, " "))
		}
		found := false
		for _, present := range flags {
			if present == flag {
				found = true
				break
			}
		}
		if !found {
			flags = append(flags, flag)
		}
	}
	sort.Strings(flags)
	return flags, style, date, nil
}

func readerValidateHeader(header map[string]string, order []string) error {
	want := []string{"TITLE", "DATE", "DATE_NUMERIC"}
	if _, ok := header["CS"]; ok {
		want = append(want, "CS")
	}
	want = append(want, "VERSION", "SEPARATOR")
	if strings.Join(order, "\x00") != strings.Join(want, "\x00") {
		return errors.New("incomplete header or invalid field order")
	}
	for _, key := range []string{"TITLE", "DATE"} {
		v := header[key]
		if len(v) < 2 || v[0] != '"' || v[len(v)-1] != '"' {
			return fmt.Errorf("invalid header field %s", key)
		}
	}
	if !readerIntegerPattern.MatchString(header["DATE_NUMERIC"]) {
		return errors.New("invalid DATE_NUMERIC")
	}
	version := header["VERSION"]
	if len(version) != 1 || version[0] < '1' || version[0] > '8' {
		return errors.New("unsupported VERSION (supported: 1–8)")
	}
	separator := []rune(header["SEPARATOR"])
	if len(separator) != 3 || separator[0] != '\'' || separator[2] != '\'' {
		return errors.New("invalid SEPARATOR")
	}
	return nil
}

func readerNonLeg(kind, text string) error {
	switch kind {
	case "XSECT":
		parts, rest, err := readerPrefix(text, 5)
		if err != nil {
			return errors.New("invalid XSECT")
		}
		for _, part := range parts[1:] {
			valid, err := readerNonLegNumber(part, true)
			if err != nil {
				return err
			}
			if !valid {
				return errors.New("invalid XSECT")
			}
		}
		_, suffix, err := readerNamed(rest)
		if err != nil {
			return err
		}
		if len(suffix) > 1 {
			return errors.New("invalid XSECT metadata")
		}
		if len(suffix) == 1 {
			return readerSurveyDate(suffix[0])
		}
	case "XSECT_END":
		if text != kind {
			return errors.New("invalid XSECT_END")
		}
	case "ERROR_INFO":
		matches := readerErrorInfoPattern.FindStringSubmatch(text)
		if matches == nil {
			return errors.New("invalid ERROR_INFO")
		}
		for _, part := range matches[1:] {
			valid, err := readerNonLegNumber(part, false)
			if err != nil {
				return err
			}
			if !valid {
				return errors.New("invalid ERROR_INFO")
			}
		}
	default:
		return fmt.Errorf("unknown record %s", kind)
	}
	return nil
}

// ParseDump validates a complete UTF-8 dump and retains original record order.
func ParseDump(r io.Reader) (*Model, error) {
	model := &Model{Header: make(map[string]string), Stations: make(map[Point]Station), InputFormat: "dump"}
	var headerOrder []string
	inBody, stopped, hasCurrent := false, false, false
	var current Point
	geometryKind := ""
	lineNumber := 0
	parseLine := func(raw string) error {
		if !utf8.ValidString(raw) {
			return errors.New("dump is not valid UTF-8")
		}
		text := strings.TrimFunc(raw, readerSpace)
		if text == "" {
			return nil
		}
		if text == "EXTENDED ELEVATION" {
			return errors.New("extended elevation: an ordinary spatial model is required")
		}
		if stopped {
			return errors.New("data after STOP (possible --rewind stream)")
		}
		if !inBody {
			if text == "--" {
				if err := readerValidateHeader(model.Header, headerOrder); err != nil {
					return err
				}
				inBody = true
				return nil
			}
			key, value, found := strings.Cut(text, " ")
			if !found {
				return errors.New("invalid header field")
			}
			switch key {
			case "TITLE", "DATE", "DATE_NUMERIC", "CS", "VERSION", "SEPARATOR":
			default:
				return errors.New("unknown or repeated header field")
			}
			if _, exists := model.Header[key]; exists {
				return errors.New("unknown or repeated header field")
			}
			model.Header[key] = value
			headerOrder = append(headerOrder, key)
			return nil
		}
		parts, _, _ := readerPrefix(text, 1)
		kind := parts[0]
		switch kind {
		case "MOVE":
			point, err := readerPoint(readerFields(text)[1:])
			if err != nil {
				return err
			}
			current = point
			hasCurrent = true
		case "LEG", "LINE":
			if geometryKind != "" && geometryKind != kind {
				return errors.New("mixed LEG and LINE records")
			}
			geometryKind = kind
			count := 7
			if kind == "LINE" {
				count = 4
			}
			fields, rest, err := readerPrefix(text, count)
			if err != nil {
				return err
			}
			var start, end Point
			if kind == "LEG" {
				start, err = readerPoint(fields[1:4])
				if err != nil {
					return err
				}
				end, err = readerPoint(fields[4:7])
			} else {
				if !hasCurrent {
					return errors.New("LINE before MOVE")
				}
				start = current
				end, err = readerPoint(fields[1:4])
				current = end
			}
			if err != nil {
				return err
			}
			survey, suffix, err := readerNamed(rest)
			if err != nil {
				return err
			}
			flags, style, date, err := readerLegSuffix(suffix)
			if err != nil {
				return err
			}
			model.Legs = append(model.Legs, Leg{ID: len(model.Legs) + 1, SourceLine: lineNumber, A: start, B: end, Survey: survey, Flags: flags, Style: style, Date: date})
		case "NODE":
			fields, rest, err := readerPrefix(text, 4)
			if err != nil {
				return err
			}
			point, err := readerPoint(fields[1:])
			if err != nil {
				return err
			}
			name, flags, err := readerNamed(rest)
			if err != nil {
				return err
			}
			station := model.Stations[point]
			for _, flag := range flags {
				switch flag {
				case "ANON":
					station.Anonymous = true
				case "SURFACE", "UNDERGROUND", "ENTRANCE", "EXPORTED", "FIXED", "WALL":
				default:
					return errors.New("unknown NODE flags")
				}
			}
			if name == "" {
				station.Anonymous = true
			} else {
				station.Names = append(station.Names, name)
			}
			model.Stations[point] = station
		case "STOP":
			if text != kind {
				return errors.New("invalid STOP")
			}
			stopped = true
		default:
			return readerNonLeg(kind, text)
		}
		return nil
	}
	buffer := bufio.NewReaderSize(r, 64*1024)
	for {
		raw, readErr := buffer.ReadString('\n')
		// Match text-file universal newlines, including an old dump using lone CR.
		for len(raw) > 0 {
			i := strings.IndexAny(raw, "\r\n")
			var line string
			if i < 0 {
				line = raw
				raw = ""
			} else {
				line = raw[:i]
				delimiter := raw[i]
				raw = raw[i+1:]
				if delimiter == '\r' && strings.HasPrefix(raw, "\n") {
					raw = raw[1:]
				}
			}
			lineNumber++
			if err := parseLine(line); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNumber, err)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("line %d: reading dump: %w", lineNumber+1, readErr)
		}
	}
	if !inBody || !stopped {
		return nil, fmt.Errorf("line %d: truncated dump: missing header or STOP", lineNumber+1)
	}
	for point, station := range model.Stations {
		sort.Strings(station.Names)
		unique := station.Names[:0]
		for _, name := range station.Names {
			if len(unique) == 0 || name != unique[len(unique)-1] {
				unique = append(unique, name)
			}
		}
		station.Names = unique
		station.Status = "unnamed"
		if len(unique) > 0 {
			station.Status = "named"
		}
		model.Stations[point] = station
	}
	return model, nil
}

// ReadModel detects binary input by its signature, then validates streamed dump3d
// output and the final process status. Stderr is disk-backed to prevent deadlock.
func ReadModel(path, inputFormat, dump3d string) (*Model, error) {
	if inputFormat != "auto" && inputFormat != "dump" && inputFormat != "3d" {
		return nil, errors.New("input_format must be auto, dump or 3d")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if inputFormat == "auto" {
		signature := make([]byte, len(readerSignature))
		n, err := io.ReadFull(file, signature)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return nil, err
		}
		inputFormat = "dump"
		if string(signature[:n]) == readerSignature {
			inputFormat = "3d"
		}
		if _, err = file.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
	}
	if inputFormat == "dump" {
		return ParseDump(file)
	}
	if dump3d == "" {
		dump3d = "dump3d"
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	errorsFile, err := os.CreateTemp("", "survex-go-dump3d-stderr-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(errorsFile.Name())
	defer errorsFile.Close()
	command := exec.Command(dump3d, "--legs", "--show-dates", abs)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		remove := key == "LC_ALL" || key == "LC_NUMERIC"
		if runtime.GOOS == "windows" {
			remove = strings.EqualFold(key, "LC_ALL") || strings.EqualFold(key, "LC_NUMERIC")
		}
		if !remove {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "LC_NUMERIC=C")
	command.Stderr = errorsFile
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = command.Start(); err != nil {
		stdout.Close()
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("dump3d not found: %s; install Survex or use --dump3d", dump3d)
		}
		return nil, fmt.Errorf("starting dump3d: %w", err)
	}
	model, parseErr := ParseDump(stdout)
	// The parser can fail before the child is finished writing; drain before Wait.
	if parseErr != nil {
		_, _ = io.Copy(io.Discard, stdout)
	}
	waitErr := command.Wait()
	if waitErr != nil {
		_, _ = errorsFile.Seek(0, io.SeekStart)
		diagnostic, _ := io.ReadAll(io.LimitReader(errorsFile, 8192))
		return nil, fmt.Errorf("dump3d exited with code %d; required options --legs --show-dates: %s", command.ProcessState.ExitCode(), strings.TrimSpace(strings.ToValidUTF8(string(diagnostic), "\ufffd")))
	}
	if parseErr != nil {
		return nil, parseErr
	}
	model.InputFormat = "3d"
	return model, nil
}
