package main

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

var toleranceDecimal = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

// NewTolerance parses the decimal exactly; Value preserves the caller's text.
func NewTolerance(mode, value string) (*Tolerance, error) {
	if mode != "absolute_m" && mode != "relative_percent" {
		return nil, fmt.Errorf("unknown tolerance mode %q", mode)
	}
	if !toleranceDecimal.MatchString(value) {
		return nil, fmt.Errorf("tolerance must be a positive, finite decimal number")
	}
	if value[0] == '-' {
		return nil, fmt.Errorf("tolerance must be a positive, finite decimal number")
	}
	// Parse the coefficient and decimal exponent explicitly. Rat.SetString has
	// an undocumented-in-the-contract exponent limit of 1e6; qualification must
	// not silently narrow the accepted finite decimal syntax to that limit.
	mantissa := strings.TrimPrefix(value, "+")
	exponent := new(big.Int)
	if e := strings.IndexAny(mantissa, "eE"); e >= 0 {
		exponent.SetString(mantissa[e+1:], 10)
		mantissa = mantissa[:e]
	}
	if dot := strings.IndexByte(mantissa, '.'); dot >= 0 {
		exponent.Sub(exponent, big.NewInt(int64(len(mantissa)-dot-1)))
		mantissa = mantissa[:dot] + mantissa[dot+1:]
	}
	coefficient, _ := new(big.Int).SetString(mantissa, 10)
	if coefficient.Sign() <= 0 {
		return nil, fmt.Errorf("tolerance must be a positive, finite decimal number")
	}
	if mode == "absolute_m" {
		exponent.Add(exponent, big.NewInt(2))
	} else {
		exponent.Sub(exponent, big.NewInt(2))
	}
	power := new(big.Int).Exp(big.NewInt(10), new(big.Int).Abs(exponent), nil)
	var scale *big.Rat
	if exponent.Sign() >= 0 {
		scale = new(big.Rat).SetInt(coefficient.Mul(coefficient, power))
	} else {
		scale = new(big.Rat).SetFrac(coefficient, power)
	}
	return &Tolerance{Mode: mode, Value: value, ScaleSq: new(big.Rat).Mul(scale, scale)}, nil
}

// SquaredDistance is exact over all validated signed int32 centimetre inputs.
// A coordinate difference fits int64; its square and the sum may not.
func SquaredDistance(a, b Point) *big.Int {
	var sum, difference big.Int
	for k := range a {
		difference.SetInt64(a[k] - b[k])
		difference.Mul(&difference, &difference)
		sum.Add(&sum, &difference)
	}
	return &sum
}

func largerSquared(a, b *big.Int) *big.Int {
	if a.Cmp(b) >= 0 {
		return a
	}
	return b
}

// MatchLegs qualifies both endpoints using exact squared centimetres. The
// orientation with the smaller maximum error wins; ties remain direct.
func MatchLegs(a, b Leg, t *Tolerance) *Match {
	e1, e2 := SquaredDistance(a.A, b.A), SquaredDistance(a.B, b.B)
	r1, r2 := SquaredDistance(a.A, b.B), SquaredDistance(a.B, b.A)
	reversed := largerSquared(r1, r2).Cmp(largerSquared(e1, e2)) < 0
	if reversed {
		e1, e2 = r1, r2
	}
	threshold := new(big.Int).Set(t.ScaleSq.Num())
	if t.Mode == "relative_percent" {
		lengthA, lengthB := SquaredDistance(a.A, a.B), SquaredDistance(b.A, b.B)
		if lengthB.Cmp(lengthA) < 0 {
			lengthA = lengthB
		}
		threshold.Mul(threshold, lengthA)
	}
	threshold.Quo(threshold, t.ScaleSq.Denom())
	if largerSquared(e1, e2).Cmp(threshold) > 0 {
		return nil
	}
	return &Match{Reversed: reversed, E1: e1, E2: e2}
}

// QueryRadius returns min(ceil(sqrt(q)), 2^32) without floating arithmetic.
// The cap spans every possible coordinate difference in a valid model.
func QueryRadius(lengthSq *big.Int, t *Tolerance) int64 {
	numerator := new(big.Int).Set(t.ScaleSq.Num())
	if t.Mode == "relative_percent" {
		numerator.Mul(numerator, lengthSq)
	}
	denominator := t.ScaleSq.Denom()
	capSquared := new(big.Int).Lsh(big.NewInt(1), 64)
	if numerator.Cmp(capSquared.Mul(capSquared, denominator)) >= 0 {
		return 1 << 32
	}
	radius := new(big.Int).Quo(numerator, denominator)
	radius.Sqrt(radius)
	square := new(big.Int).Mul(radius, radius)
	square.Mul(square, denominator)
	result := radius.Int64()
	if square.Cmp(numerator) < 0 {
		result++
	}
	return result
}
