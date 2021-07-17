package topology

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	topo, err := ParseFile("testdata/leafspine-nomgmt.dot")
	if err != nil {
		t.Fatal(err)
	}
	if xs := topo.Devices(); len(xs) != 5 {
		t.Errorf("got %d devices, want 5", len(xs))
	}
	if xs := topo.Links(); len(xs) != 6 {
		t.Errorf("got %d links, want 6", len(xs))
	}
	for _, x := range topo.Devices() {
		wantFunction := Leaf
		if x.Name == "spine0" || x.Name == "spine1" {
			wantFunction = Spine
		}
		if f := x.Function(); f != wantFunction {
			t.Errorf("device %s: got function %s, want %s",
				x.Name, f, wantFunction)
		}
	}
}

func TestAutoMgmtNetwork(t *testing.T) {
	topo, err := ParseFile("testdata/leafspine.dot", WithAutoMgmtNetwork)
	if err != nil {
		t.Fatal(err)
	}
	if xs := topo.Devices(); len(xs) != 8 {
		t.Errorf("got %d devices, want 8", len(xs))
	}
	if xs := topo.Links(); len(xs) != 15 {
		t.Errorf("got %d links, want 15", len(xs))
	}
}

const invalidHostnamesDOT = `graph G {
	"t" [function=tor]
	"h_with_underscore" [function=host]
	"t":swp1 -- "h_with_underscore":eth0
}
`

func TestInvalidHostnames(t *testing.T) {
	_, err := Parse([]byte(invalidHostnamesDOT))
	if err == nil || !strings.Contains(err.Error(), "invalid hostname") {
		t.Errorf("got err=%v, want invalid hostname", err)
	}
}

// Test that we handle multiple parallel links between two devices.
func TestParallelLinks(t *testing.T) {
	const G = `
		graph G {
			"a":swp2 -- "b":swp2
			"a":swp1 -- "b":swp1
		}`

	topo, err := Parse([]byte(G))
	if err != nil {
		t.Fatal(err)
	}

	if xs := topo.Links(); len(xs) != 2 {
		t.Errorf("got %d links, want 2", len(xs))
	}
}
