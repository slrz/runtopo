package topology

import (
	"fmt"
	"io/ioutil"

	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/encoding/dot"
	"inet.af/netaddr"
)

// T represents a parsed network topology graph.
type T struct {
	g    *dotGraph
	devs map[string]*Device
	dot  []byte

	autoMgmt  bool
	mgmtLinks []Link
}

// Option may be passed to Parse to customize topology processing.
type Option func(*T)

// WithAutoMgmtNetwork enables an out-of-band management network. The topology
// is augmented with a management switch and server, with the latter running
// DHCP and DNS services for all devices. Devices are automatically attached to
// the management switch unless they have the no_mgmt node attribute set.
var WithAutoMgmtNetwork = func(t *T) {
	t.autoMgmt = true
}

// Parse unmarshals a DOT graph. It returns the topology described by it or an
// error, if any.
func Parse(dotBytes []byte, opts ...Option) (*T, error) {
	g := newDotGraph()
	if err := dot.Unmarshal(dotBytes, g); err != nil {
		return nil, fmt.Errorf("Parse: %w", err)
	}
	t := &T{g: g, devs: make(map[string]*Device)}
	for _, opt := range opts {
		opt(t)
	}

	for _, d := range t.devices() {
		if !isValidHostname(d.Name) {
			return nil, fmt.Errorf("invalid hostname: %q", d.Name)
		}
		d := d
		t.devs[d.Name] = &d
	}
	if t.autoMgmt {
		if err := t.setupAutoMgmtNetwork(); err != nil {
			return nil, err
		}
	}

	// associate links with their endpoints
	for _, l := range t.Links() {
		l := l
		if t.devs[l.From] == nil || t.devs[l.To] == nil {
			if l.From != "oob-mgmt-server" && l.From != "oob-mgmt-switch" ||
				l.FromPort != "eth0" || l.To != "" {
				return nil, fmt.Errorf("edge has unknown nodes: %s", l)
			}
		}
		t.devs[l.From].links = append(
			t.devs[l.From].links, l,
		)
		if l.To != "" {
			t.devs[l.To].links = append(
				t.devs[l.To].links, l,
			)
		}

	}

	// Stash a copy of the input DOT graph for later use.
	t.dot = make([]byte, len(dotBytes))
	copy(t.dot, dotBytes)

	return t, nil
}

// ParseFile is like Parse but reads the DOT graph description from the file
// located by path.
func ParseFile(path string, opts ...Option) (*T, error) {
	p, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ParseFile: %w", err)
	}
	return Parse(p, opts...)
}

// Devices returns the set of devices defined in the topology.
func (t *T) Devices() []Device {
	var ds []Device
	for _, p := range t.devs {
		ds = append(ds, *p)
	}
	return ds
}

func (t *T) devices() []Device {
	var ds []Device
	for _, n := range graph.NodesOf(t.g.Nodes()) {
		n := n.(*dotNode)
		ds = append(ds, Device{
			Name:  n.dotID,
			attrs: n.attrs,
		})
	}
	return ds
}

// Links returns the connections between devices, as defined in the topology.
func (t *T) Links() []Link {
	var ls []Link
	for _, e := range graph.EdgesOf(t.g.Edges()) {
		e := e.(*dotEdge)
		fromPort, _ := e.FromPort()
		toPort, _ := e.ToPort()
		ls = append(ls, Link{
			From:     e.From().(*dotNode).dotID,
			FromPort: fromPort,
			To:       e.To().(*dotNode).dotID,
			ToPort:   toPort,
			attrs:    e.attrs,
		})
	}
	return append(ls, t.mgmtLinks...)
}

// DOT returns the original input DOT file.
func (t *T) DOT() []byte {
	return append([]byte(nil), t.dot...)
}

func (t *T) setupAutoMgmtNetwork() error {
	mgmtServer := t.devs["oob-mgmt-server"]
	if mgmtServer == nil {
		mgmtServer = &Device{
			Name: "oob-mgmt-server",
			attrs: map[string]string{
				"function": OOBServer.String(),
				"mgmt_ip":  "192.168.200.254/24",
			},
		}
		t.devs["oob-mgmt-server"] = mgmtServer
	}
	mgmtServerUplink := Link{
		From:     "oob-mgmt-server",
		FromPort: "eth0",
	}
	t.mgmtLinks = append(t.mgmtLinks, mgmtServerUplink)

	mgmtSwitch := t.devs["oob-mgmt-switch"]
	if mgmtSwitch == nil {
		mgmtSwitch = &Device{
			Name: "oob-mgmt-switch",
			attrs: map[string]string{
				"function": OOBSwitch.String(),
			},
		}
		t.devs["oob-mgmt-switch"] = mgmtSwitch
	}
	mgmtSwitchMgmtLink := Link{
		From:     "oob-mgmt-switch",
		FromPort: "eth0",
	}
	mgmtLink := Link{
		From:     "oob-mgmt-server",
		FromPort: "eth1",
		To:       "oob-mgmt-switch",
		ToPort:   "swp1",
	}
	t.mgmtLinks = append(t.mgmtLinks, mgmtSwitchMgmtLink, mgmtLink)

	mgmtPrefix, err := netaddr.ParseIPPrefix(mgmtServer.Attr("mgmt_ip"))
	if err != nil {
		return err
	}
	a := newIPAllocator(mgmtPrefix)
	a.reserve(mgmtPrefix.IP) // remove mgmtServer's own address
	// reserve addresses configured with explicit node attrs
	for _, d := range t.devs {
		if HasFunction(d, OOBSwitch, OOBServer, Fake) {
			continue
		}
		ipStr := d.Attr("mgmt_ip")
		if ipStr == "" || d.Attr("no_mgmt") != "" {
			continue
		}
		ip, err := netaddr.ParseIP(ipStr)
		if err != nil {
			return fmt.Errorf("device %s: parse ip: %v (mgmt_ip: %s)",
				d.Name, err, ipStr)
		}
		if ok := a.reserve(ip); !ok {
			return fmt.Errorf("device %s: unable to reserve ip %s",
				d.Name, ip)
		}
		d.mgmtIP = ip
	}

	// Wire up devices to the OOB switch.
	ifIndex := 2
	for _, d := range t.devs {
		if d.Attr("no_mgmt") != "" {
			continue
		}
		if HasFunction(d, OOBSwitch, OOBServer, Fake) {
			continue
		}
		l := Link{
			From:     mgmtSwitch.Name,
			FromPort: fmt.Sprintf("swp%d", ifIndex),
			To:       d.Name,
			ToPort:   "eth0",
		}
		t.mgmtLinks = append(t.mgmtLinks, l)
		ifIndex++

		if !d.mgmtIP.IsZero() {
			// has explicit mgmt_ip attr, address reserved above
			continue
		}
		ip, ok := a.allocate()
		if !ok {
			return fmt.Errorf(
				"device %s: mgmt ip range exhausted (prefix: %s)",
				d.Name, mgmtPrefix)
		}
		d.mgmtIP = ip
	}

	return nil
}
