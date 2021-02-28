package libvirt

import (
	"sort"
	"testing"
)

func TestNatSort(t *testing.T) {
	for testNo, test := range natSortTests {
		a := make([]string, len(test.input))
		copy(a, test.input)
		sort.Slice(a, func(i, j int) bool {
			return natCompare(a[i], a[j]) < 0
		})
		for i, s := range a {
			if s != test.golden[i] {
				t.Errorf("case %d: want a[%d] == %s, got %s",
					testNo, i, test.golden[i], s)
			}
		}
	}
}

// These test vectors were taken from Dave Koelle's website describing
// Alphanum. http://www.davekoelle.com/alphanum.html
var natSortTests = []struct {
	input, golden []string
}{
	{
		input: []string{
			"z102.doc",
			"z12.doc",
			"z5.doc",
			"z9.doc",
			"z16.doc",
			"z10.doc",
			"z15.doc",
			"z4.doc",
			"z17.doc",
			"z3.doc",
			"z100.doc",
			"z8.doc",
			"z14.doc",
			"z1.doc",
			"z19.doc",
			"z11.doc",
			"z6.doc",
			"z20.doc",
			"z18.doc",
			"z2.doc",
			"z101.doc",
			"z7.doc",
			"z13.doc",
		},
		golden: []string{
			"z1.doc",
			"z2.doc",
			"z3.doc",
			"z4.doc",
			"z5.doc",
			"z6.doc",
			"z7.doc",
			"z8.doc",
			"z9.doc",
			"z10.doc",
			"z11.doc",
			"z12.doc",
			"z13.doc",
			"z14.doc",
			"z15.doc",
			"z16.doc",
			"z17.doc",
			"z18.doc",
			"z19.doc",
			"z20.doc",
			"z100.doc",
			"z101.doc",
			"z102.doc",
		},
	},
	{
		input: []string{
			"1000X Radonius Maximus",
			"10X Radonius",
			"200X Radonius",
			"20X Radonius",
			"20X Radonius Prime",
			"30X Radonius",
			"40X Radonius",
			"Allegia 50 Clasteron",
			"Allegia 500 Clasteron",
			"Allegia 50B Clasteron",
			"Allegia 51 Clasteron",
			"Allegia 6R Clasteron",
			"Alpha 100",
			"Alpha 2",
			"Alpha 200",
			"Alpha 2A",
			"Alpha 2A-8000",
			"Alpha 2A-900",
			"Callisto Morphamax",
			"Callisto Morphamax 500",
			"Callisto Morphamax 5000",
			"Callisto Morphamax 600",
			"Callisto Morphamax 6000 SE",
			"Callisto Morphamax 6000 SE2",
			"Callisto Morphamax 700",
			"Callisto Morphamax 7000",
			"Xiph Xlater 10000",
			"Xiph Xlater 2000",
			"Xiph Xlater 300",
			"Xiph Xlater 40",
			"Xiph Xlater 5",
			"Xiph Xlater 50",
			"Xiph Xlater 500",
			"Xiph Xlater 5000",
			"Xiph Xlater 58",
		},
		golden: []string{
			"10X Radonius",
			"20X Radonius",
			"20X Radonius Prime",
			"30X Radonius",
			"40X Radonius",
			"200X Radonius",
			"1000X Radonius Maximus",
			"Allegia 6R Clasteron",
			"Allegia 50 Clasteron",
			"Allegia 50B Clasteron",
			"Allegia 51 Clasteron",
			"Allegia 500 Clasteron",
			"Alpha 2",
			"Alpha 2A",
			"Alpha 2A-900",
			"Alpha 2A-8000",
			"Alpha 100",
			"Alpha 200",
			"Callisto Morphamax",
			"Callisto Morphamax 500",
			"Callisto Morphamax 600",
			"Callisto Morphamax 700",
			"Callisto Morphamax 5000",
			"Callisto Morphamax 6000 SE",
			"Callisto Morphamax 6000 SE2",
			"Callisto Morphamax 7000",
			"Xiph Xlater 5",
			"Xiph Xlater 40",
			"Xiph Xlater 50",
			"Xiph Xlater 58",
			"Xiph Xlater 300",
			"Xiph Xlater 500",
			"Xiph Xlater 2000",
			"Xiph Xlater 5000",
			"Xiph Xlater 10000",
		},
	},
}
