// Shared records; coordinates are exact signed int32 centimetres in int64 points.
package main

import "math/big"

type Point [3]int64

type Leg struct {
	ID, SourceLine int
	A, B           Point
	Survey         string
	Flags          []string
	Style, Date    string
}

type Station struct {
	Names     []string
	Anonymous bool
	Status    string
}

type Model struct {
	Legs        []Leg
	Stations    map[Point]Station
	Header      map[string]string
	InputFormat string
}

type Match struct {
	Reversed bool
	E1, E2   *big.Int
}

// A and B are zero-based indexes in the original model, before selection.
type Pair struct {
	A, B  int
	Match Match
}

type Tolerance struct {
	Mode, Value string
	ScaleSq     *big.Rat
}
