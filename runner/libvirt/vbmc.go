package libvirt

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
)

type bmc struct {
	Addr     string `json:"addr" yaml:"addr"`
	User     string `json:"user" yaml:"user"`
	Password string `json:"password" yaml:"password"`
}

type bmcConfig struct {
	connect  string // libvirt connection URI
	addr     string // virtual BMC local address
	port0    int    // virtual BMC local port base
	user     string // IPMI user
	password string // IPMI pass
}

type bmcMan struct {
	all      map[string]*bmc
	nextPort int

	// immutable after initialization
	connect  string
	addr     string
	user     string
	password string
}

func newBMCMan(c *bmcConfig) *bmcMan {
	m := &bmcMan{
		all:      make(map[string]*bmc),
		nextPort: 6230,
		connect:  "qemu:///system",
		addr:     "::",
		user:     "runtopo",
		password: randomString(16),
	}
	if v := c.connect; v != "" {
		m.connect = v
	}
	if v := c.addr; v != "" {
		m.addr = v
	}
	if v := c.port0; v != 0 {
		m.nextPort = v
	}
	if v := c.user; v != "" {
		m.user = v
	}
	if v := c.password; v != "" {
		m.password = v
	}

	return m
}

func (m *bmcMan) add(domName string) (*bmc, error) {
	if x := m.all[domName]; x != nil {
		return x, fmt.Errorf("add bmc for %s: already exists", domName)
	}

	port := m.nextPort
	m.nextPort++
	x := &bmc{
		Addr:     net.JoinHostPort(m.addr, strconv.Itoa(port)),
		User:     m.user,
		Password: m.password,
	}
	m.all[domName] = x

	return x, nil
}

func (m *bmcMan) startAll(ctx context.Context) (err error) {
	var added []string
	defer func() {
		if err != nil && len(added) > 0 {
			m.vbmcDelete(ctx, added...)
		}
	}()
	for k, v := range m.all {
		if err := m.vbmcAdd(ctx, k, v); err != nil {
			return fmt.Errorf("vbmcAdd %s: %v", k, err)
		}
		added = append(added, k)
	}

	if len(added) == 0 {
		return nil
	}
	return m.vbmcStart(ctx, added...)
}

func (m *bmcMan) stopAll(ctx context.Context) (err error) {
	var names []string
	for k := range m.all {
		names = append(names, k)
	}

	if len(names) == 0 {
		return nil
	}
	if err := m.vbmcStop(ctx, names...); err != nil {
		return err
	}
	return m.vbmcDelete(ctx, names...)
}

func (m *bmcMan) vbmcAdd(ctx context.Context, domName string, bmc *bmc) error {
	host, port, err := net.SplitHostPort(bmc.Addr)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "vbmc", "add",
		"--libvirt-uri", m.connect,
		"--address", host,
		"--port", port,
		"--username", bmc.User,
		"--password", bmc.Password,
		domName,
	)
	_, err = cmd.Output()
	if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
		err = fmt.Errorf("%v [stderr: %s]", ee, ee.Stderr)
	}

	return err
}

func (m *bmcMan) vbmcCommand(ctx context.Context, cmd string, domNames ...string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("run vbmc %s %q: %w", cmd, domNames, err)
		}
	}()
	_, err = exec.CommandContext(ctx, "vbmc",
		append([]string{cmd}, domNames...)...).Output()
	if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
		err = fmt.Errorf("%v [stderr: %s]", ee, ee.Stderr)
	}
	return err
}

func (m *bmcMan) vbmcDelete(ctx context.Context, domNames ...string) error {
	return m.vbmcCommand(ctx, "delete", domNames...)
}

func (m *bmcMan) vbmcStart(ctx context.Context, domNames ...string) error {
	return m.vbmcCommand(ctx, "start", domNames...)
}

func (m *bmcMan) vbmcStop(ctx context.Context, domNames ...string) error {
	return m.vbmcCommand(ctx, "stop", domNames...)
}
