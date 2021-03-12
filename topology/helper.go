package topology

import "inet.af/netaddr"

// An ipAllocator assigns addresses from a given prefix.
type ipAllocator struct {
	builder netaddr.IPSetBuilder
}

// NewIPAllocator constructs an ipAllocator for the prefix p.
func newIPAllocator(p netaddr.IPPrefix) *ipAllocator {
	a := new(ipAllocator)
	a.builder.AddPrefix(p)
	a.builder.Remove(p.Masked().IP) // remove network addr
	a.builder.Remove(p.Range().To)  // remove bcast addr
	return a
}

// Reserve reserves ip, preventing it from getting allocated. If ip is not in
// prefix or was returned from allocate already, reserve returns false. A
// successful reservation returns true.
func (a *ipAllocator) reserve(ip netaddr.IP) bool {
	if !a.builder.IPSet().Contains(ip) {
		return false
	}
	a.builder.Remove(ip)
	return true
}

// Allocate returns the next ip available for assignment and a boolean
// indicating success. It returnns false if the space available for allocation
// is exhausted.
func (a *ipAllocator) allocate() (netaddr.IP, bool) {
	for _, r := range a.builder.IPSet().Ranges() {
		ip := r.From
		a.builder.Remove(ip)
		return ip, true
	}

	return netaddr.IP{}, false
}

func isASCIIAlpha(c rune) bool {
	return 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z'
}

func isASCIIAlNum(c rune) bool {
	return isASCIIAlpha(c) || '0' <= c && c <= '9'
}

func isASCIIAlNumHyp(c rune) bool {
	return isASCIIAlNum(c) || c == '-'
}

func isValidHostname(s string) bool {
	if len(s) == 0 {
		return false
	}
	if !isASCIIAlpha(rune(s[0])) {
		return false
	}
	for _, c := range s[1 : len(s)-1] {
		if !isASCIIAlNumHyp(c) {
			return false
		}
	}
	return isASCIIAlNum(rune(s[len(s)-1]))
}
