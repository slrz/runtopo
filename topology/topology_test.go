package topology

import "testing"

func TestParse(t *testing.T) {
	topo, err := ParseFile("testdata/leafspine-nomgmt.dot")
	if err != nil {
		t.Fatal(err)
	}
	if xs := topo.Devices(); len(xs) != 10 {
		t.Errorf("got %d devices, want 10", len(xs))
	}
	if xs := topo.Links(); len(xs) != 16 {
		t.Errorf("got %d links, want 16", len(xs))
	}
	for _, x := range topo.Devices() {
		wantFunction := FunctionLeaf
		if x.Name == "spine0" || x.Name == "spine1" {
			wantFunction = FunctionSpine
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
	if xs := topo.Devices(); len(xs) != 12 {
		t.Errorf("got %d devices, want 12", len(xs))
	}
	if xs := topo.Links(); len(xs) != 29 {
		t.Errorf("got %d links, want 29", len(xs))
	}
}
