package topology

import (
	"testing"

	"inet.af/netaddr"
)

var testPrefixTestNet2 = netaddr.MustParseIPPrefix("198.51.100.0/29")

func TestIPAllocate(t *testing.T) {
	a := newIPAllocator(testPrefixTestNet2)
	want := netaddr.MustParseIP("198.51.100.1")
	for i := 0; i < 6; i++ {
		got, ok := a.allocate()
		if !ok {
			t.Fatalf("got %d allocations from %s, want 6",
				i+1, testPrefixTestNet2)
		}
		if got != want {
			t.Errorf("got %s, want %s", got, want)
		}
		want = want.Next()
	}
	if ip, ok := a.allocate(); ok {
		t.Errorf("got allocation %s despite exhausted range", ip)
	}
}

func TestIPReserve(t *testing.T) {
	a := newIPAllocator(testPrefixTestNet2)
	want := netaddr.MustParseIP("198.51.100.1")
	a.reserve(want)
	want = want.Next()
	for i := 0; i < 5; i++ {
		got, ok := a.allocate()
		if !ok {
			t.Fatalf("got %d allocations from %s, want 6",
				i+1, testPrefixTestNet2)
		}
		if got != want {
			t.Errorf("got %s, want %s", got, want)
		}
		want = want.Next()
	}
	if ip, ok := a.allocate(); ok {
		t.Errorf("got allocation %s despite exhausted range", ip)
	}
}
