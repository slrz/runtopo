package libvirt

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"

	libvirtxml "libvirt.org/libvirt-go-xml"
)

//go:embed domain.xml.in
var domainTemplateText string

type domainTemplateArgs struct {
	Name      string
	VCPUs     int
	Memory    int64
	Pool      string
	Image     string
	BaseImage string

	Interfaces []domainInterface
}

type domainInterface struct {
	Type      string
	MACAddr   string
	TargetDev string
	Model     string

	NetworkSource string
	UDPSource     udpSource
}
type udpSource struct {
	Address      string
	Port         uint
	LocalAddress string
	LocalPort    uint
}

var templateFuncs = template.FuncMap{
	"marshalInterface": func(in domainInterface) string {
		src := new(libvirtxml.DomainInterfaceSource)
		switch in.Type {
		case "network":
			src.Network = &libvirtxml.DomainInterfaceSourceNetwork{
				Network: in.NetworkSource,
			}
		case "udp":
			src.UDP = &libvirtxml.DomainInterfaceSourceUDP{
				Address: in.UDPSource.Address,
				Port:    in.UDPSource.Port,
				Local: &libvirtxml.DomainInterfaceSourceLocal{
					Address: in.UDPSource.LocalAddress,
					Port:    in.UDPSource.LocalPort,
				},
			}
		}
		intf := &libvirtxml.DomainInterface{
			MAC: &libvirtxml.DomainInterfaceMAC{
				Address: in.MACAddr,
			},
			Source: src,
			Model: &libvirtxml.DomainInterfaceModel{
				Type: in.Model,
			},
		}
		theXML, err := intf.Marshal()
		if err != nil {
			panic(err)
		}
		return theXML
	},
}

const udevRulesTemplateText = `{{ range . }}
ACTION=="add", SUBSYSTEM=="net", ATTR{address}=="{{ .MACAddr }}", NAME="{{ .Interface }}", SUBSYSTEMS=="pci"
{{ end }}`

var udevRulesTemplate = template.Must(template.New("").Parse(udevRulesTemplateText))

func renderUdevRules(d *device) (p []byte, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("render-udev-rules: %w", err)
		}
	}()

	type item struct {
		Interface string
		MACAddr   string
	}
	var args []item
	for _, intf := range d.interfaces {
		args = append(args, item{
			Interface: intf.name,
			MACAddr:   intf.mac.String(),
		})
	}

	var buf bytes.Buffer
	if err := udevRulesTemplate.Execute(&buf, args); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
