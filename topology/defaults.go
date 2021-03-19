package topology

type deviceDefaults struct {
	OS     string `yaml:"os"`
	VCPUs  int    `yaml:"vcpus"`
	Memory int64  `yaml:"memory"`
}

const (
	cumulusQCOW2 = "https://d2cd9e7ca6hntp.cloudfront.net/public/CumulusLinux-4.3.0/cumulus-linux-4.3.0-vx-amd64-qemu.qcow2"
	fedoraQCOW2  = "https://download.fedoraproject.org/pub/fedora/linux/releases/33/Cloud/x86_64/images/Fedora-Cloud-Base-33-1.2.x86_64.qcow2"
)

var builtinDefaults = [...]deviceDefaults{
	FunctionOOBServer:  {OS: fedoraQCOW2, VCPUs: 1, Memory: 768 << 20},
	FunctionOOBSwitch:  {OS: cumulusQCOW2, VCPUs: 1, Memory: 768 << 20},
	FunctionExit:       {OS: cumulusQCOW2, VCPUs: 1, Memory: 768 << 20},
	FunctionSuperSpine: {OS: cumulusQCOW2, VCPUs: 1, Memory: 768 << 20},
	FunctionSpine:      {OS: cumulusQCOW2, VCPUs: 1, Memory: 768 << 20},
	FunctionLeaf:       {OS: cumulusQCOW2, VCPUs: 1, Memory: 768 << 20},
	FunctionTOR:        {OS: cumulusQCOW2, VCPUs: 1, Memory: 768 << 20},
	FunctionHost:       {OS: fedoraQCOW2, VCPUs: 1, Memory: 768 << 20},
	NoFunction:         {OS: fedoraQCOW2, VCPUs: 1, Memory: 768 << 20},
}
