package libvirt

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"text/template"

	"slrz.net/runtopo/topology"

	libvirtxml "libvirt.org/libvirt-go-xml"
)

func TestValidDomainXML(t *testing.T) {
	topo, err := topology.ParseFile("testdata/leafspine.dot", topology.WithAutoMgmtNetwork)
	if err != nil {
		t.Fatal(err)
	}

	r := NewRunner()
	if err := r.buildInventory(topo); err != nil {
		t.Fatal(err)
	}

	tmpl, err := template.New("").
		Funcs(templateFuncs).
		Parse(domainTemplateText)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	for _, d := range r.devices {
		if err := tmpl.Execute(&buf, d.templateArgs()); err != nil {
			t.Errorf("domain %s: %v", d.name, err)
		}
		domXML := buf.Bytes()
		if err := validateDomainXML(domXML); err != nil {
			t.Errorf("domain %s: %v", d.name, err)
		}
		buf.Reset()
	}
}

func TestDnsmasqHostsFile(t *testing.T) {
	topo, err := topology.ParseFile("testdata/leafspine.dot", topology.WithAutoMgmtNetwork)
	if err != nil {
		t.Fatal(err)
	}

	r := NewRunner()
	if err := r.buildInventory(topo); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	content := generateDnsmasqHostsFile(gatherHosts(ctx, r, topo))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(content), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	ndevices := 0
	for _, d := range topo.Devices() {
		if d.Attr("no_mgmt") != "" {
			continue
		}
		if topology.HasFunction(&d, topology.OOBServer,
			topology.OOBSwitch, topology.Fake) {
			continue
		}
		ndevices++
	}

	if n := len(lines); n != ndevices {
		t.Errorf("got %d entries, want %d", n, ndevices)
	}

	for i, l := range lines {
		xs := strings.Split(l, ",")
		if len(xs) != 3 {
			t.Errorf("line %d invalid: %q\n", i+1, l)
		}
	}
}

func TestDomainPXEBoot(t *testing.T) {
	topo, err := topology.ParseFile("testdata/pxehost.dot")
	if err != nil {
		t.Fatal(err)
	}

	r := NewRunner()
	if err := r.buildInventory(topo); err != nil {
		t.Fatal(err)
	}

	tmpl, err := template.New("").
		Funcs(templateFuncs).
		Parse(domainTemplateText)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	d := r.devices["host1"]
	if err := tmpl.Execute(&buf, d.templateArgs()); err != nil {
		t.Errorf("domain %s: %v", d.name, err)
	}
	dom := new(libvirtxml.Domain)
	if err := dom.Unmarshal(buf.String()); err != nil {
		t.Fatalf("domain %s: %v", d.name, err)
	}
	disks := dom.Devices.Disks
	if len(disks) == 0 {
		t.Fatalf("domain %s: no disks", d.name)
	}
	if d0 := disks[0]; d0.Boot == nil || d0.Boot.Order != 2 {
		t.Fatalf("domain %s: unexpected boot order: %#v", d.name, d0.Boot)
	}
	pxeOK := false
	for _, intf := range dom.Devices.Interfaces {
		if intf.Boot != nil && intf.Boot.Order == 1 {
			pxeOK = true
		}
	}
	if !pxeOK {
		t.Fatalf("domain %s: no interface configured for booting", d.name)
	}
}
