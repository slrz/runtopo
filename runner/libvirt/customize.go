package libvirt

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"golang.org/x/crypto/bcrypt"
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
		"--hostname", d.topoDev.Name,
		"--timezone", "Etc/UTC",
		"--write", "/etc/udev/rules.d/70-persistent-net.rules:"+string(rules),
		"--commands-from-file", "/dev/stdin",
	)
	cmd.Stdin = io.MultiReader(extraCommands, bytes.NewReader(commandsForFunction(d)))

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (stderr: %s)", err, out)
	}

	return nil
}

func commandsForFunction(d *device) []byte {
	var buf bytes.Buffer
	if f := d.topoDev.Function(); isCumulusFunction(f) {
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
		if f == topology.FunctionOOBSwitch {
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
	buf.WriteString("install dnsmasq,lldpd\n")
	buf.WriteString("run-command systemctl enable lldpd.service\n")

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

	// From virt-customize(1): […] arguments can be spread across multiple
	// lines, by adding a "\" (continuation character) at the of a line […]
	io.WriteString(w, "write /etc/network/interfaces.d/bridge.intf:"+
		strings.Replace(bridgeConf, "\n", "\\\n", -1)+"\n")
}

