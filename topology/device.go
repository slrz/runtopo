package topology

import (
	"net"
	"strconv"

	"inet.af/netaddr"
)

// A Device corresponds to a node in the parsed topology.
type Device struct {
	Name   string
	attrs  map[string]string
	links  []Link
	mgmtIP netaddr.IP
}

// Function returns the DeviceFunction associated with d.
func (d *Device) Function() DeviceFunction {
	return deviceFunctionFromString(d.attrs["function"])
}

// VCPUs returns the number of CPUs requested for a device ('cpu' node
// attribute) or a function-specific default.
func (d *Device) VCPUs() int {
	if s := d.Attr("cpu"); s != "" {
		n, err := strconv.Atoi(s)
		if err == nil {
			return n
		}
	}
	return builtinDefaults[d.Function()].VCPUs
}

// Memory returns the device's memory size in bytes.
func (d *Device) Memory() int64 {
	if s := d.Attr("memory"); s != "" {
		n, err := strconv.ParseInt(s, 0, 64)
		if err == nil {
			// node attribute "memory" is in MiB, we want bytes.
			return n << 20
		}
	}
	return builtinDefaults[d.Function()].Memory
}

// DiskSize returns the device's disk size in bytes.
func (d *Device) DiskSize() int64 {
	if s := d.Attr("disk"); s != "" {
		n, err := strconv.ParseInt(s, 0, 64)
		if err == nil {
			// node attribute "disk" is in GiB, we want bytes.
			return n << 30
		}
	}
	return 8 << 30
}

// OSImage returns the URL to an operating system image from the 'os' node
// attribute, falling back to a builtin default if necessary.
func (d *Device) OSImage() string {
	if s := d.Attr("os"); s != "" {
		if s == "none" {
			return ""
		}
		return s
	}
	return builtinDefaults[d.Function()].OS
}

// MgmtIP returns the management IP address assigned to d (only when
// AutoMgmtNetwork is configured).
func (d *Device) MgmtIP() *net.IPAddr {
	if d.mgmtIP.IsZero() {
		return nil
	}
	return d.mgmtIP.IPAddr()
}

// Links returns all connections involving d as an endpoint.
func (d *Device) Links() []Link {
	ls := make([]Link, len(d.links))
	copy(ls, d.links)
	return ls
}

// Attr returns the node attribute associated with key, if any.
func (d *Device) Attr(key string) string {
	return d.attrs[key]
}

// DeviceFunction describes a device's role in the topology and is used for
// startup ordering as well as determining default OS images.
type DeviceFunction int

// NOTE: do not change the string representations, it'd break compatibility
// with existing DOT files and topology_converter.

//go:generate stringer -type=DeviceFunction -linecomment
const (
	Fake       DeviceFunction = iota // fake
	OOBServer                        // oob-server
	OOBSwitch                        // oob-switch
	Exit                             // exit
	SuperSpine                       // superspine
	Spine                            // spine
	Leaf                             // leaf
	TOR                              // tor
	Host                             // host
	NoFunction
)

func deviceFunctionFromString(s string) DeviceFunction {
	switch s {
	case "fake":
		return Fake
	case "oob-server":
		return OOBServer
	case "oob-switch":
		return OOBSwitch
	case "exit":
		return Exit
	case "superspine":
		return SuperSpine
	case "spine":
		return Spine
	case "leaf":
		return Leaf
	case "tor":
		return TOR
	case "host":
		return Host
	default:
		return NoFunction
	}
}

// HasFunction returns whether d.Function() is in fs.
func HasFunction(d *Device, fs ...DeviceFunction) bool {
	want := d.Function()
	for _, f := range fs {
		if f == want {
			return true
		}
	}
	return false
}
