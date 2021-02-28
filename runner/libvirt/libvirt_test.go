package libvirt

import (
	"bytes"
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
			t.Errorf("domain %s: %w", d.name, err)
		}
		domXML := buf.Bytes()
		if err := validateDomainXML(domXML); err != nil {
			t.Errorf("domain %s: %w", d.name, err)
		}
		buf.Reset()
	}
}
