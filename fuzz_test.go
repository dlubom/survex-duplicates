package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func qualityVerifiedBytes(t testing.TB, path, want string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != want {
		t.Fatalf("snapshot hash mismatch: %s", path)
	}
	return data
}

func FuzzParseDump(f *testing.F) {
	provenance := qualityVerifiedBytes(f, filepath.Join("tests", "fixtures", "provenance.json"),
		"282ad9665dc31ce37f013369cfb262c522021c5f41dfd220c1a513f9951fd11a")
	var saved struct {
		SHA256 map[string]string `json:"sha256"`
	}
	if err := json.Unmarshal(provenance, &saved); err != nil {
		f.Fatal(err)
	}
	for _, name := range []string{"survey.legs.dump", "survey.lines.dump", "legacy.legs.dump", "closure.legs.dump", "extended.legs.dump"} {
		f.Add(qualityVerifiedBytes(f, filepath.Join("tests", "fixtures", name), saved.SHA256[name]))
	}
	for _, body := range []string{
		"NODE 0 0 0 [named] ANON\nNODE 0 0 0 [alias]\nLEG 0 0 0 1 0 0 []",
		"NODE 0 0 0 [p],\"q[]\nLEG 0 0 0 1 0 0 [p],\"q[] SURFACE SPLAY SURFACE",
		"LEG -21474836.48 +0.000 1e-2 21474836.47 .0 -1E-2 []",
		"LEG 0.001 0 0 1 0 0 []",
		"\nMOVE 0 0 0\n\nNODE 0 0 0 [] ANON\nLINE 1 0 0 []\n",
		"LEG 0 0 0 1 0 0 [] 2024.02.29-2024.03.01",
		"",
	} {
		for _, newline := range []string{"\n", "\r\n", "\r"} {
			f.Add([]byte(strings.ReplaceAll(readerFullDump(body), "\n", newline)))
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		// Bound the fuzz workload only; the application has no such input limit.
		if len(data) > 64*1024 {
			t.Skip()
		}
		model, err := ParseDump(bytes.NewReader(data))
		if err != nil {
			return
		}
		if model == nil || model.InputFormat != "dump" || model.Header == nil || model.Stations == nil {
			t.Fatalf("incomplete successful model: %+v", model)
		}
		lastLine := 0
		for i, leg := range model.Legs {
			if leg.ID != i+1 || leg.SourceLine <= lastLine {
				t.Fatalf("invalid ordered identity: %+v", leg)
			}
			lastLine = leg.SourceLine
			qualityCheckPoint(t, leg.A)
			qualityCheckPoint(t, leg.B)
			qualityCheckSortedUnique(t, leg.Flags)
		}
		for point, station := range model.Stations {
			qualityCheckPoint(t, point)
			qualityCheckSortedUnique(t, station.Names)
			wantStatus := "unnamed"
			if len(station.Names) > 0 {
				wantStatus = "named"
			}
			if station.Status != wantStatus || len(station.Names) == 0 && !station.Anonymous {
				t.Fatalf("inconsistent station at %v: %+v", point, station)
			}
		}
	})
}

func qualityCheckPoint(t *testing.T, point Point) {
	t.Helper()
	for _, coordinate := range point {
		if coordinate < -2147483648 || coordinate > 2147483647 {
			t.Fatalf("coordinate outside signed int32 centimetres: %v", point)
		}
	}
}

func qualityCheckSortedUnique(t *testing.T, values []string) {
	t.Helper()
	for i, value := range values {
		if value == "" || i > 0 && values[i-1] >= value {
			t.Fatalf("values are not nonempty, sorted and distinct: %q", values)
		}
	}
}

// Cyclic decoding makes even short mutations exercise real geometry. Records
// deliberately include repeated, reversed, shifted, point and independent legs.
// At most ten legs keeps the independent rational oracle inexpensive.
func qualityFuzzModel(data []byte) *Model {
	if len(data) == 0 {
		data = []byte{0}
	}
	at := func(i int) byte { return data[i%len(data)] }
	word := func(i int) uint32 {
		return binary.LittleEndian.Uint32([]byte{at(i), at(i + 1), at(i + 2), at(i + 3)})
	}
	model := &Model{}
	for i, n := 0, 2+int(at(0)%9); i < n; i++ {
		offset := 4 + i*26
		leg := Leg{ID: i + 1, SourceLine: i + 7}
		for axis := 0; axis < 3; axis++ {
			a, b := int64(int32(word(offset+axis*4))), int64(int32(word(offset+12+axis*4)))
			if at(3)&1 == 0 {
				a, b = int64(int16(a)), int64(int16(b))
			}
			leg.A[axis], leg.B[axis] = a, b
		}
		if i > 0 {
			previous := model.Legs[i-1]
			switch at(offset+24) % 6 {
			case 0:
				leg.A, leg.B = previous.A, previous.B
			case 1:
				leg.A, leg.B = previous.B, previous.A
			case 2:
				leg.A, leg.B = previous.A, previous.B
				// Keep near-boundary offsets on the signed int32 grid.
				delta := int64(at(offset+25)%13) - 6
				if leg.A[1]+delta >= -2147483648 && leg.A[1]+delta <= 2147483647 && leg.B[1]+delta >= -2147483648 && leg.B[1]+delta <= 2147483647 {
					leg.A[1] += delta
					leg.B[1] += delta
				}
			case 3:
				leg.B = leg.A
			case 4:
				leg.A, leg.B = previous.A, previous.B
				leg.A[0], leg.B[0] = leg.B[0], leg.A[0]
			}
		}
		if at(offset+25)&1 != 0 {
			leg.Flags = append(leg.Flags, "SPLAY")
		}
		if at(offset+25)&2 != 0 {
			leg.Flags = append(leg.Flags, "DUPLICATE")
		}
		if at(offset+25)&4 != 0 {
			leg.Flags = append(leg.Flags, "SURFACE")
		}
		model.Legs = append(model.Legs, leg)
	}
	return model
}

func FuzzSearchOracle(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{8, 0, 0, 0}) // Ten identical eligible point legs.
	f.Add([]byte{8, 1, 3, 1, 0, 0, 0, 128, 255, 255, 255, 127})
	// An exact 3-4-5 boundary, followed by a reversed and a shifted record.
	seed := make([]byte, 4+4*26)
	seed[0] = 2
	seed[1], seed[2] = 1, 2 // Absolute 5 cm, including acknowledged duplicates.
	for i := 0; i < 4; i++ {
		offset := 4 + i*26
		binary.LittleEndian.PutUint32(seed[offset+12:], 100)
		seed[offset+24] = byte([]int{5, 5, 1, 2}[i])
	}
	for _, offset := range []int{4 + 26, 4 + 26 + 12} {
		binary.LittleEndian.PutUint32(seed[offset:], uint32(3))
		binary.LittleEndian.PutUint32(seed[offset+4:], uint32(4))
	}
	binary.LittleEndian.PutUint32(seed[4+26+12:], uint32(103))
	seed[4+3*26+25] = 11 // Shift by five; flags exercise both selection policies.
	f.Add(seed)
	for i := byte(0); i < 8; i++ {
		f.Add([]byte{7, i, i, i, 100, 0, 3, 4, 5, 6, 255, 127})
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 512 {
			// Retain a meaningful bounded model even for large mutations.
			data = data[:512]
		}
		control := func(i int) byte {
			if len(data) == 0 {
				return 0
			}
			return data[i%len(data)]
		}
		modes := []struct{ mode, value string }{
			{"relative_percent", "5"}, {"absolute_m", "0.05"},
			{"relative_percent", "4.9999"}, {"absolute_m", "0.0099"},
			{"relative_percent", "1000"}, {"absolute_m", "1e12"},
			{"relative_percent", "100"}, {"absolute_m", "0.01"},
		}
		mode := modes[int(control(1))%len(modes)]
		value, ok := new(big.Rat).SetString(mode.value)
		if !ok {
			t.Fatal("invalid fixed rational")
		}
		tolerance := mustTolerance(t, mode.mode, mode.value)
		excludeSplays, includeDuplicates := control(2)&1 != 0, control(2)&2 != 0
		batch := 1 + int(control(3)%13)
		check := func(model *Model) []exactTuple {
			t.Helper()
			want := exhaustiveOracle(model, mode.mode, value, excludeSplays, includeDuplicates)
			search := NewSearch(model, tolerance, excludeSplays, includeDuplicates)
			for _, size := range []int{1, batch, len(model.Legs) + 1} {
				got := collectedPairs(t, search, size)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("%s=%s filters=%v/%v batch=%d\ngot %v\nwant %v\nmodel %+v", mode.mode, mode.value, excludeSplays, includeDuplicates, size, got, want, model.Legs)
				}
			}
			return want
		}
		model := qualityFuzzModel(data)
		original := check(model)
		translated := &Model{Legs: append([]Leg(nil), model.Legs...)}
		for axis := 0; axis < 3; axis++ {
			lo, hi := int64(2147483647), int64(-2147483648)
			for _, leg := range model.Legs {
				lo = min(lo, leg.A[axis], leg.B[axis])
				hi = max(hi, leg.A[axis], leg.B[axis])
			}
			delta := min(int64(100000000), 2147483647-hi)
			if delta == 0 {
				delta = -min(int64(100000000), lo+2147483648)
			}
			for i := range translated.Legs {
				translated.Legs[i].A[axis] += delta
				translated.Legs[i].B[axis] += delta
			}
		}
		if got := check(translated); !reflect.DeepEqual(got, original) {
			t.Fatalf("translation changed complete tuples: %v versus %v", got, original)
		}
		reversed := &Model{Legs: append([]Leg(nil), model.Legs...)}
		for i := range reversed.Legs {
			if i%2 == 0 {
				reversed.Legs[i].A, reversed.Legs[i].B = reversed.Legs[i].B, reversed.Legs[i].A
			}
		}
		// On orientation ties, reversing one leg can select different endpoint
		// errors. Membership is invariant; full orientation/errors use the oracle.
		if got := check(reversed); !reflect.DeepEqual(qualityPairIdentities(got), qualityPairIdentities(original)) {
			t.Fatalf("endpoint reversal changed pairs: %v versus %v", got, original)
		}
		permuted := &Model{Legs: append([]Leg(nil), model.Legs...)}
		mapping := make([]int, len(model.Legs))
		for i := range mapping {
			mapping[i] = i
		}
		for i := len(mapping) - 1; i > 0; i-- {
			j := int(control(i+4)) % (i + 1)
			mapping[i], mapping[j] = mapping[j], mapping[i]
		}
		for i, old := range mapping {
			permuted.Legs[i] = model.Legs[old]
			permuted.Legs[i].ID = i + 1
		}
		remapped := check(permuted)
		for i := range remapped {
			p := &remapped[i]
			p.a, p.b = mapping[p.a], mapping[p.b]
			if p.a > p.b {
				p.a, p.b = p.b, p.a
				if p.reversed {
					p.e1, p.e2 = p.e2, p.e1
				}
			}
		}
		sort.Slice(remapped, func(i, j int) bool {
			return remapped[i].a < remapped[j].a || remapped[i].a == remapped[j].a && remapped[i].b < remapped[j].b
		})
		if !reflect.DeepEqual(remapped, original) {
			t.Fatalf("permutation changed identity-mapped tuples: %v versus %v", remapped, original)
		}
	})
}

func qualityPairIdentities(pairs []exactTuple) [][2]int {
	var identities [][2]int
	for _, pair := range pairs {
		identities = append(identities, [2]int{pair.a, pair.b})
	}
	return identities
}
