package libvirt

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"inet.af/netaddr"
	"slrz.net/runtopo/topology"
)

func customizeDomain(ctx context.Context, uri string, d *device, extraCommands io.Reader) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("customizeDomain %s: %w", d.name, err)
		}
	}()

	if extraCommands == nil {
		extraCommands = eofReader{}
	}

	rules, err := renderUdevRules(d)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "virt-customize", "-q",
		"-d", d.name,
		"-c", uri,
		"--hostname", d.Name,
		"--timezone", "Etc/UTC",
		// This rename script basically does s/eth/swp/ and breaks
		// proper interface naming using udev rules. Delete it.
		"--delete", "/etc/hw_init.d/S10rename_eth_swp.sh",
		"--write", "/etc/udev/rules.d/70-persistent-net.rules:"+string(rules),
		"--commands-from-file", "/dev/stdin",
	)
	commands := []io.Reader{extraCommands}
	if len(d.config) > 0 {
		file, err := writeTempFile("", d.name+"-config", d.config)
		if err != nil {
			return err
		}
		defer os.Remove(file)
		commands = append(commands, strings.NewReader("run "+file+"\n"))
	}

	cmd.Stdin = io.MultiReader(append(commands,
		// As the commands returned from commandsForFunction may
		// contain selinux-relabel, they need to come last, after any
		// other operation touching the guest file system. The commands
		// are basically required for proper functioning anyway, so
		// there's not much of an use case for overriding them.
		bytes.NewReader(commandsForFunction(d)))...,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (stderr: %s)", err, out)
	}

	return nil
}

func commandsForFunction(d *device) []byte {
	var buf bytes.Buffer
	if hasCumulusFunction(d) {
		// These eat enough memory to summon the OOM killer in 512MiB
		// VMs.
		buf.WriteString("run-command systemctl disable netq-agent.service\n")
		buf.WriteString("run-command systemctl disable netqd@mgmt.service\n")
		buf.WriteString("run-command passwd -x 99999 cumulus\n") // CL4+
		buf.WriteString("write /etc/sudoers.d/no-passwd:%sudo     ALL=(ALL:ALL) NOPASSWD: ALL\n")
		// Set password for user cumulus to some random string.
		// Otherwise, CL4+ forces a password change on first login.
		cryptPW, err := bcrypt.GenerateFromPassword([]byte(randomString(16)), -1)
		if err != nil {
			panic(err) // something is very wrong if this happens
		}
		fmt.Fprintf(&buf, "run-command usermod -p %s cumulus\n", cryptPW)

		// libguestfs (1.44) thinks it doesn't know how to set
		// hostnames for CL. Work around by directly writing to
		// /etc/hostname.
		fmt.Fprintf(&buf, "write /etc/hostname:%s\\\n\n", d.Name)
		if d.Function() == topology.OOBSwitch {
			writeExtraMgmtSwitchCommands(&buf, d)
		}
		return buf.Bytes()
	}

	var cloudInitUnits = []string{
		"cloud-init.service",
		"cloud-init-local.service",
		"cloud-config.service",
		"cloud-final.service",
	}
	// We use cloud images but don't provide the VMs with any cloud init
	// configuration source. Disable cloud-init or it will block the boot.
	for _, u := range cloudInitUnits {
		buf.WriteString("run-command systemctl disable " + u + "\n")
	}
	buf.WriteString("install lldpd\n")
	buf.WriteString("run-command systemctl enable lldpd.service\n")
	// Make lldpd emit the interface name instead of the MAC address. It's
	// what we have in the topology file.
	buf.WriteString("write /etc/lldpd.d/ifname.conf:configure lldp portidsubtype ifname\\\n\n")

	if d.Function() == topology.OOBServer {
		writeExtraMgmtServerCommands(&buf, d)
	}
	// Only required for SELinux-enabled systems (mostly Fedora/EL)
	buf.WriteString("selinux-relabel\n")

	return buf.Bytes()
}

func writeExtraMgmtSwitchCommands(w io.Writer, d *device) {
	var bridgePorts []string
	for _, intf := range d.interfaces {
		if intf.name == "eth0" {
			// skip mgmt interface
			continue
		}
		bridgePorts = append(bridgePorts, intf.name)
	}
	bridgeConf := "auto bridge\niface bridge\n    bridge-ports " +
		strings.Join(bridgePorts, " ") + "\n"

	// From virt-customize(1): [???] arguments can be spread across multiple
	// lines, by adding a "\" (continuation character) at the of a line [???]
	io.WriteString(w, "write /etc/network/interfaces.d/bridge.intf:"+
		strings.Replace(bridgeConf, "\n", "\\\n", -1)+"\n")
}

const (
	nftablesRuleset = `
table ip nat {
	chain postrouting {
		type nat hook postrouting priority srcnat; policy accept;
		masquerade
	}
}
`
	dnsmasqConf = `
strict-order
interface=eth1
dhcp-range=%s,static
dhcp-no-override
dhcp-authoritative
dhcp-hostsfile=/etc/dnsmasq.hostsfile
`

	ifcfgEth1 = `
TYPE=Ethernet
DEVICE=eth1
ONBOOT=yes
BOOTPROTO=none
IPADDR=%s
PREFIX=%d
`
)

func writeExtraMgmtServerCommands(w io.Writer, d *device) {
	io.WriteString(w, "install nftables,dnsmasq\n")
	// We assume that the prefix has already been validated.
	p := netaddr.MustParseIPPrefix(d.Attr("mgmt_ip"))
	io.WriteString(w, "write /etc/sysconfig/network-scripts/ifcfg-eth0:"+
		"TYPE=Ethernet\\\nDEVICE=eth0\\\nPEERDNS=yes\\\nBOOTPROTO=dhcp\\\nONBOOT=yes\n")
	io.WriteString(w, "write /etc/sysconfig/network-scripts/ifcfg-eth1:"+
		strings.Replace(
			fmt.Sprintf(ifcfgEth1, p.IP, p.Bits),
			"\n", "\\\n", -1,
		)+"\n",
	)
	io.WriteString(w, "write /etc/sysconfig/nftables.conf:"+
		strings.Replace(nftablesRuleset, "\n", "\\\n", -1)+"\n")

	io.WriteString(w, "run-command systemctl enable nftables.service\n")
	io.WriteString(w, "write /etc/sysctl.d/98-ipfwd.conf:net.ipv4.ip_forward=1\n")
	io.WriteString(w, "write /etc/dnsmasq.conf:"+
		strings.Replace(fmt.Sprintf(dnsmasqConf, p.Masked().IP),
			"\n", "\\\n", -1)+"\n")
	io.WriteString(w, "run-command systemctl disable systemd-resolved.service\n")
	// Ensure /etc/resolv.conf is a regular file (and not a symlink to
	// systemd-resolved's stub-resolv.conf). Dnsmasq reads its upstream
	// resolvers from resolv.conf and we need NM to write the ones received
	// from DHCP there.
	io.WriteString(w, "delete /etc/resolv.conf\n")
	io.WriteString(w, "write /etc/resolv.conf:#placeholder\n")
	io.WriteString(w, "run-command systemctl enable dnsmasq.service\n")
}

type etherHost struct {
	name string
	ip   *net.IPAddr
	mac  net.HardwareAddr
}

func gatherHosts(ctx context.Context, r *Runner, t *topology.T) []etherHost {
	var hosts []etherHost
	for name, d := range r.devices {
		if name == "oob-mgmt-server" || name == "oob-mgmt-switch" {
			continue
		}
		eth0 := d.interfaces[0]
		if eth0.name != "eth0" {
			// most likely, device does not have a mgmt interface
			continue
		}
		mgmtIP := d.MgmtIP()
		if mgmtIP == nil {
			continue
		}
		hosts = append(hosts, etherHost{
			name: name,
			ip:   mgmtIP,
			mac:  eth0.mac,
		})
	}

	return hosts
}

func generateDnsmasqHostsFile(hosts []etherHost) []byte {
	var buf bytes.Buffer
	for _, h := range hosts {
		fmt.Fprintf(&buf, "%s,%s,%s\n", h.mac, h.ip, h.name)
	}
	return buf.Bytes()
}
