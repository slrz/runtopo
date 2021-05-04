package topology

type deviceDefaults struct {
	OS     string `yaml:"os"`
	VCPUs  int    `yaml:"vcpus"`
	Memory int64  `yaml:"memory"`
}

const (
	cumulusQCOW2 = "https://d2cd9e7ca6hntp.cloudfront.net/public/CumulusLinux-4.3.0/cumulus-linux-4.3.0-vx-amd64-qemu.qcow2"
	fedoraQCOW2  = "https://download.fedoraproject.org/pub/fedora/linux/releases/34/Cloud/x86_64/images/Fedora-Cloud-Base-34-1.2.x86_64.qcow2"
)

var builtinDefaults = [...]deviceDefaults{
	OOBServer:  {OS: fedoraQCOW2, VCPUs: 1, Memory: 768 << 20},
	OOBSwitch:  {OS: cumulusQCOW2, VCPUs: 1, Memory: 768 << 20},
	Exit:       {OS: cumulusQCOW2, VCPUs: 1, Memory: 768 << 20},
	SuperSpine: {OS: cumulusQCOW2, VCPUs: 1, Memory: 768 << 20},
	Spine:      {OS: cumulusQCOW2, VCPUs: 1, Memory: 768 << 20},
	Leaf:       {OS: cumulusQCOW2, VCPUs: 1, Memory: 768 << 20},
	TOR:        {OS: cumulusQCOW2, VCPUs: 1, Memory: 768 << 20},
	Host:       {OS: fedoraQCOW2, VCPUs: 1, Memory: 768 << 20},
	NoFunction: {OS: fedoraQCOW2, VCPUs: 1, Memory: 768 << 20},
}
