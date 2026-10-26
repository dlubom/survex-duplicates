package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var reportHeader = strings.Split("leg_a,leg_b,survey_a,survey_b,source_line_a,source_line_b,distance_m,endpoint_1_error_m,endpoint_2_error_m,reversed,flags_a,flags_b,style_a,style_b,date_a,date_b,a_x1,a_y1,a_z1,a_x2,a_y2,a_z2,b_x1,b_y1,b_z1,b_x2,b_y2,b_z2,distance_cm,length_a_m,length_b_m,relative_error_percent,tolerance_mode,tolerance_value,tolerance_m,a1_station_names,a2_station_names,b1_station_names,b2_station_names,a1_station_status,a2_station_status,b1_station_status,b2_station_status,a1_has_anonymous_node,a2_has_anonymous_node,b1_has_anonymous_node,b2_has_anonymous_node", ",")

// reportDecimal retains coefficient scale, including trailing zeros, so report
// values have Python Decimal's representation rather than a rational fraction.
type reportDecimal struct {
	coefficient *big.Int
	exponent    int64
	negative    bool
}

func reportAddExponent(a, b int64) (int64, error) {
	if b > 0 && a > math.MaxInt64-b || b < 0 && a < math.MinInt64-b {
		return 0, fmt.Errorf("decimal exponent is out of range")
	}
	return a + b, nil
}

func parseReportDecimal(text string) (reportDecimal, error) {
	if !toleranceDecimal.MatchString(text) {
		return reportDecimal{}, fmt.Errorf("invalid decimal number %q", text)
	}
	negative := strings.HasPrefix(text, "-")
	text = strings.TrimPrefix(strings.TrimPrefix(text, "+"), "-")
	var exponent int64
	if split := strings.IndexAny(text, "eE"); split >= 0 {
		var err error
		exponent, err = strconv.ParseInt(text[split+1:], 10, 64)
		if err != nil {
			return reportDecimal{}, fmt.Errorf("decimal exponent is out of range: %w", err)
		}
		text = text[:split]
	}
	if point := strings.IndexByte(text, '.'); point >= 0 {
		var err error
		exponent, err = reportAddExponent(exponent, -int64(len(text)-point-1))
		if err != nil {
			return reportDecimal{}, err
		}
		text = text[:point] + text[point+1:]
	}
	coefficient, ok := new(big.Int).SetString(text, 10)
	if !ok {
		return reportDecimal{}, fmt.Errorf("invalid decimal coefficient")
	}
	return reportDecimal{coefficient, exponent, negative}, nil
}

func (d reportDecimal) text() (string, error) {
	digits := d.coefficient.String()
	adjusted, err := reportAddExponent(d.exponent, int64(len(digits))-1)
	if err != nil {
		return "", err
	}
	prefix := ""
	if d.negative {
		prefix = "-"
	}
	if d.exponent <= 0 && adjusted >= -6 {
		point := int64(len(digits)) + d.exponent
		if point <= 0 {
			return prefix + "0." + strings.Repeat("0", int(-point)) + digits, nil
		}
		if point == int64(len(digits)) {
			return prefix + digits, nil
		}
		return prefix + digits[:point] + "." + digits[point:], nil
	}
	if len(digits) > 1 {
		digits = digits[:1] + "." + digits[1:]
	}
	suffix := strconv.FormatInt(adjusted, 10)
	if adjusted >= 0 {
		suffix = "+" + suffix
	}
	return prefix + digits + "E" + suffix, nil
}

// CanonicalDecimal mirrors str(Decimal(text)) for validated CLI parameters.
// Invalid text is left intact; validation belongs to NewTolerance.
func CanonicalDecimal(text string) string {
	d, err := parseReportDecimal(text)
	if err != nil {
		return text
	}
	canonical, err := d.text()
	if err != nil {
		return text
	}
	return canonical
}

func reportPower10(exponent int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
}

// roundReportDecimal applies decimal round-half-even once, preserving exact
// values' scale. Each Python Decimal arithmetic operation rounds separately.
func roundReportDecimal(d reportDecimal, precision int) (reportDecimal, error) {
	excess := len(d.coefficient.String()) - precision
	if excess <= 0 {
		return d, nil
	}
	divisor := reportPower10(excess)
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(d.coefficient, divisor, remainder)
	comparison := remainder.Lsh(remainder, 1).Cmp(divisor)
	if comparison > 0 || comparison == 0 && quotient.Bit(0) == 1 {
		quotient.Add(quotient, big.NewInt(1))
	}
	exponent, err := reportAddExponent(d.exponent, int64(excess))
	if err != nil {
		return reportDecimal{}, err
	}
	if len(quotient.String()) > precision {
		quotient.Quo(quotient, big.NewInt(10))
		exponent, err = reportAddExponent(exponent, 1)
		if err != nil {
			return reportDecimal{}, err
		}
	}
	return reportDecimal{quotient, exponent, d.negative}, nil
}

// sqrtReportInteger computes Decimal(integer).sqrt() with correctly rounded
// decimal digits. Squared distances contain at most 66 bits. Perfect squares
// retain exponent zero; irrational roots use exactly the requested precision.
func sqrtReportInteger(value *big.Int, precision int) reportDecimal {
	root := new(big.Int).Sqrt(value)
	if new(big.Int).Mul(root, root).Cmp(value) == 0 {
		return reportDecimal{coefficient: root}
	}
	adjusted := (len(value.String()) - 1) / 2
	scale := precision - 1 - adjusted
	scaled := new(big.Int).Mul(value, reportPower10(2*scale))
	root.Sqrt(scaled)
	// sqrt(scaled) >= root + 1/2 exactly when 4*scaled >= (2*root+1)^2.
	midpoint := new(big.Int).Lsh(new(big.Int).Set(root), 1)
	midpoint.Add(midpoint, big.NewInt(1))
	midpoint.Mul(midpoint, midpoint)
	comparison := scaled.Lsh(scaled, 2).Cmp(midpoint)
	if comparison > 0 || comparison == 0 && root.Bit(0) == 1 {
		root.Add(root, big.NewInt(1))
	}
	return reportDecimal{coefficient: root, exponent: -int64(scale)}
}

func reportToleranceMetres(value reportDecimal, shortest *big.Int) (string, error) {
	precision := max(40, len(value.coefficient.String())+20)
	root := sqrtReportInteger(shortest, precision)
	exponent, err := reportAddExponent(value.exponent, root.exponent)
	if err != nil {
		return "", err
	}
	product, err := roundReportDecimal(reportDecimal{
		coefficient: new(big.Int).Mul(value.coefficient, root.coefficient),
		exponent:    exponent,
		negative:    value.negative,
	}, precision)
	if err != nil {
		return "", err
	}
	// Division by the exact integer 10000 needs no additional rounding. Python
	// removes up to four coefficient zeros to approach the preferred exponent.
	if product.coefficient.Sign() != 0 {
		product.exponent, err = reportAddExponent(product.exponent, -4)
		if err != nil {
			return "", err
		}
		for i := 0; i < 4; i++ {
			quotient, remainder := new(big.Int), new(big.Int)
			quotient.QuoRem(product.coefficient, big.NewInt(10), remainder)
			if remainder.Sign() != 0 {
				break
			}
			product.coefficient = quotient
			product.exponent++
		}
	}
	return product.text()
}

func reportFloat(value float64) string { return strconv.FormatFloat(value, 'g', 17, 64) }

func reportIntegerFloat(value *big.Int) float64 {
	result, _ := new(big.Float).SetInt(value).Float64()
	return result
}

func reportMetres(cm int64) string {
	sign := ""
	if cm < 0 {
		sign = "-"
		cm = -cm
	}
	return fmt.Sprintf("%s%d.%02d", sign, cm/100, cm%100)
}

func reportBoolean(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

// Python's ensure_ascii=False emits Unicode literally, including U+2028 and
// U+2029; encoding/json would escape those even with HTML escaping disabled.
func reportNames(names []string) string {
	var result strings.Builder
	result.WriteByte('[')
	for i, name := range names {
		if i > 0 {
			result.WriteString(", ")
		}
		result.WriteByte('"')
		for _, r := range name {
			switch r {
			case '\\', '"':
				result.WriteByte('\\')
				result.WriteRune(r)
			case '\b':
				result.WriteString(`\b`)
			case '\f':
				result.WriteString(`\f`)
			case '\n':
				result.WriteString(`\n`)
			case '\r':
				result.WriteString(`\r`)
			case '\t':
				result.WriteString(`\t`)
			default:
				if r < 0x20 {
					fmt.Fprintf(&result, `\u%04x`, r)
				} else {
					result.WriteRune(r)
				}
			}
		}
		result.WriteByte('"')
	}
	result.WriteByte(']')
	return result.String()
}

func reportRow(model *Model, pair Pair, tolerance *Tolerance, decimal reportDecimal, valueText string) ([]string, error) {
	if pair.A < 0 || pair.A >= len(model.Legs) || pair.B < 0 || pair.B >= len(model.Legs) {
		return nil, fmt.Errorf("invalid report pair indexes")
	}
	a, b := model.Legs[pair.A], model.Legs[pair.B]
	points := [4]Point{a.A, a.B, b.A, b.B}
	lengthA, lengthB := SquaredDistance(a.A, a.B), SquaredDistance(b.A, b.B)
	shortest := lengthA
	if lengthB.Cmp(shortest) < 0 {
		shortest = lengthB
	}
	distanceSq := pair.Match.E1
	if pair.Match.E2.Cmp(distanceSq) > 0 {
		distanceSq = pair.Match.E2
	}
	distance := math.Sqrt(reportIntegerFloat(distanceSq))
	allowed := valueText
	if tolerance.Mode == "relative_percent" {
		var err error
		allowed, err = reportToleranceMetres(decimal, shortest)
		if err != nil {
			return nil, err
		}
	}
	row := []string{
		strconv.Itoa(a.ID), strconv.Itoa(b.ID), a.Survey, b.Survey,
		strconv.Itoa(a.SourceLine), strconv.Itoa(b.SourceLine),
		reportFloat(distance / 100), reportFloat(math.Sqrt(reportIntegerFloat(pair.Match.E1)) / 100), reportFloat(math.Sqrt(reportIntegerFloat(pair.Match.E2)) / 100),
		reportBoolean(pair.Match.Reversed), strings.Join(a.Flags, "|"), strings.Join(b.Flags, "|"), a.Style, b.Style, a.Date, b.Date,
	}
	for _, point := range points {
		for _, coord := range point {
			row = append(row, reportMetres(coord))
		}
	}
	relative := ""
	if shortest.Sign() != 0 {
		// Python integer true division rounds the exact rational directly to
		// float64; converting the numerator and denominator first can differ.
		ratio, _ := new(big.Rat).SetFrac(distanceSq, shortest).Float64()
		relative = reportFloat(100 * math.Sqrt(ratio))
	}
	row = append(row, reportFloat(distance), reportFloat(math.Sqrt(reportIntegerFloat(lengthA))/100), reportFloat(math.Sqrt(reportIntegerFloat(lengthB))/100), relative, tolerance.Mode, valueText, allowed)
	var stations [4]Station
	for i, point := range points {
		station, ok := model.Stations[point]
		if !ok {
			station.Status = "missing_node"
		}
		stations[i] = station
		row = append(row, reportNames(station.Names))
	}
	for _, station := range stations {
		row = append(row, station.Status)
	}
	for _, station := range stations {
		row = append(row, reportBoolean(station.Anonymous))
	}
	return row, nil
}

// Match csv.writer's minimal quoting and CRLF without Go's extra quoting of
// leading whitespace or its newline transformation inside quoted fields.
func writeReportRecord(writer io.StringWriter, fields []string) error {
	var record strings.Builder
	for i, field := range fields {
		if i > 0 {
			record.WriteByte(',')
		}
		if strings.ContainsAny(field, ",\"\r\n") {
			record.WriteByte('"')
			record.WriteString(strings.ReplaceAll(field, "\"", "\"\""))
			record.WriteByte('"')
		} else {
			record.WriteString(field)
		}
	}
	record.WriteString("\r\n")
	_, err := writer.WriteString(record.String())
	return err
}

// WriteReport streams into a private file in the destination directory. Only
// complete, flushed output is published; Link provides atomic no-clobber
// semantics even if another process creates the destination during search.
func WriteReport(path string, model *Model, search *Search, tolerance *Tolerance, batchSize int, force bool) error {
	if model == nil || search == nil || tolerance == nil {
		return fmt.Errorf("missing report input")
	}
	decimal, err := parseReportDecimal(tolerance.Value)
	if err != nil {
		return err
	}
	valueText, err := decimal.text()
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	defer file.Close()
	writer := bufio.NewWriterSize(file, 64*1024)
	if err := writeReportRecord(writer, reportHeader); err != nil {
		return err
	}
	if err := search.Pairs(batchSize, func(pair Pair) error {
		row, err := reportRow(model, pair, tolerance, decimal, valueText)
		if err != nil {
			return err
		}
		return writeReportRecord(writer, row)
	}); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if force {
		return os.Rename(temporary, path)
	}
	return os.Link(temporary, path)
}

func resolveReportPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	parent := filepath.Dir(absolute)
	if parent == absolute {
		return absolute, nil
	}
	resolvedParent, err := resolveReportPath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolvedParent, filepath.Base(absolute)), nil
}

func ValidateDestination(source, destination string, force bool) error {
	sourcePath, err := resolveReportPath(source)
	if err != nil {
		return err
	}
	destinationPath, err := resolveReportPath(destination)
	if err != nil {
		return err
	}
	if sourcePath == destinationPath {
		return fmt.Errorf("output must not refer to the input file, including through a symlink or hardlink")
	}
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return err
	}
	destinationInfo, err := os.Stat(destination)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil && os.SameFile(sourceInfo, destinationInfo) {
		return fmt.Errorf("output must not refer to the input file, including through a symlink or hardlink")
	}
	if !force {
		_, err := os.Lstat(destination)
		if err == nil {
			return fmt.Errorf("output already exists; use --force to replace it")
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
