package topology

import (
	"fmt"
	"net"
)

// A Link corresponds to an edge in the input graph and describes a
// point-to-point connection between two devices.
type Link struct {
	From     string
	FromPort string
	To       string
	ToPort   string

	attrs map[string]string
}

// FromMAC returns the MAC address corresponding to FromPort. If the topology
// does not specify such address, the boolean return value is false.
func (l *Link) FromMAC() (net.HardwareAddr, bool) {
	if s := l.attrs["left_mac"]; s != "" {
		mac, err := net.ParseMAC(s)
		return mac, err == nil
	}

	return nil, false
}

// ToMAC returns the MAC address corresponding to ToPort. If the topology
// does not specify such address, the boolean return value is false.
func (l *Link) ToMAC() (net.HardwareAddr, bool) {
	if s := l.attrs["right_mac"]; s != "" {
		mac, err := net.ParseMAC(s)
		return mac, err == nil
	}

	return nil, false
}

// Attr returns the value associated with the attribute key.
func (l *Link) Attr(key string) string {
	return l.attrs[key]
}

// String returns a string representation of l.
func (l *Link) String() string {
	return fmt.Sprintf("%s:%s -- %s:%s",
		l.From, l.FromPort, l.To, l.ToPort)
}
