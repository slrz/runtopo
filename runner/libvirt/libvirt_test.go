package libvirt

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"text/template"

	"slrz.net/runtopo/topology"
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
		if f := d.Function(); f == topology.FunctionOOBServer ||
			f == topology.FunctionOOBSwitch {
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
