package topology

import "strconv"

// A Device corresponds to a node in the parsed topology.
type Device struct {
	Name  string
	attrs map[string]string
	links []Link
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
			return n >> 20
		}
	}
	return builtinDefaults[d.Function()].Memory
}

// OSImage returns the URL to an operating system image from the 'os' node
// attribute, falling back to a builtin default if necessary.
func (d *Device) OSImage() string {
	if s := d.Attr("os"); s != "" {
		return s
	}
	return builtinDefaults[d.Function()].OS
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
	FunctionFake       DeviceFunction = iota // fake
	FunctionOOBServer                        // oob-server
	FunctionOOBSwitch                        // oob-switch
	FunctionExit                             // exit
	FunctionSuperSpine                       // superspine
	FunctionSpine                            // spine
	FunctionLeaf                             // leaf
	FunctionTOR                              // tor
	FunctionHost                             // host
	NoFunction
)

func deviceFunctionFromString(s string) DeviceFunction {
	switch s {
	case "fake":
		return FunctionFake
	case "oob-server":
		return FunctionOOBServer
	case "oob-switch":
		return FunctionOOBSwitch
	case "exit":
		return FunctionExit
	case "superspine":
		return FunctionSuperSpine
	case "spine":
		return FunctionSpine
	case "leaf":
		return FunctionLeaf
	case "tor":
		return FunctionTOR
	case "host":
		return FunctionHost
	default:
		return NoFunction
	}
}
