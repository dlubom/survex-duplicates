package main

import (
	"reflect"
	"strings"
	"testing"
)

// A named NODE carrying ANON must retain both independent pieces of metadata.
// Dropping ANON on named records loses information even when aliases coexist.
func TestReaderNamedANONRetainsNamesAndAnonymousIndicator(t *testing.T) {
	body := "NODE 0 0 0 [named] ANON\nNODE 0 0 0 [alias]\nNODE 0 0 0 [named]"
	model, err := ParseDump(strings.NewReader(readerFullDump(body)))
	if err != nil {
		t.Fatal(err)
	}
	station := model.Stations[Point{}]
	if station.Status != "named" || !reflect.DeepEqual(station.Names, []string{"alias", "named"}) || !station.Anonymous {
		t.Fatalf("named ANON metadata lost: %+v", station)
	}
}

// Source identities count the complete dump, including empty and whitespace
// lines. Counting parsed records instead silently changes report identity.
func TestReaderSourceLinesIncludeBlankAndWhitespaceLines(t *testing.T) {
	body := "\nNODE 0 0 0 [named]\n \t\nLEG 0 0 0 1 0 0 []\n\nLEG 0 0 0 1 0 0 []"
	for _, newline := range []string{"\n", "\r\n", "\r"} {
		t.Run(strings.ReplaceAll(strings.ReplaceAll(newline, "\r", "CR"), "\n", "LF"), func(t *testing.T) {
			dump := strings.ReplaceAll(readerFullDump(body), "\n", newline)
			model, err := ParseDump(strings.NewReader(dump))
			if err != nil {
				t.Fatal(err)
			}
			if len(model.Legs) != 2 || model.Legs[0].ID != 1 || model.Legs[1].ID != 2 || model.Legs[0].SourceLine != 10 || model.Legs[1].SourceLine != 12 {
				t.Fatalf("full-stream identities not retained: %+v", model.Legs)
			}
		})
	}
}
