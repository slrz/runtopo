// Command runtopo starts up a network topology as described by the DOT file
// provided as a positional argument.
package main

import (
	"context"
	"flag"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"slrz.net/runtopo/runner/libvirt"
	"slrz.net/runtopo/topology"
)

var (
	libvirtURI = flag.String("c", os.Getenv("LIBVIRT_DEFAULT_URI"),
		"connect to specified `URI`")
	macAddrBase = flag.String("macbase", os.Getenv("RUNTOPO_MAC_BASE"),
		"auto-assigned MAC addresses start at `base`")
	namePrefix = flag.String("nameprefix",
		getEnvOrDefault("RUNTOPO_NAME_PREFIX", "runtopo-"),
		"prefix names of created resources with `string`")
	tunnelIP = flag.String("tunnelip",
		getEnvOrDefault("RUNTOPO_TUNNEL_IP", "127.0.0.1"),
		"set the default `address` for UDP tunnels")
	portBase = flag.Int("portbase", atoi(getEnvOrDefault("RUNTOPO_PORT_BASE", "10000")),
		"start allocating UDP ports at `base` instead of the default")
	portGap = flag.Int("portgap", atoi(getEnvOrDefault("RUNTOPO_PORT_GAP", "1000")),
		"leave `num` ports between local and remote side")
	autoMgmt = flag.Bool("automgmt", os.Getenv("RUNTOPO_AUTO_MGMT") != "",
		"create automagic management network")
	storagePool = flag.String("pool",
		getEnvOrDefault("RUNTOPO_LIBVIRT_POOL", "default"),
		"store downloaded base and created diff images in libvirt storage `pool`")
	writeSSHConfig = flag.String("writesshconfig",
		os.Getenv("RUNTOPO_WRITE_SSH_CONFIG"),
		"write OpenSSH client configuration to `file`")
	writeBMCConfig = flag.String("writebmcconfig",
		os.Getenv("RUNTOPO_WRITE_BMC_CONFIG"),
		"write JSON `file` containing virtual BMC addresses")
	bmcAddr = flag.String("bmcaddr",
		os.Getenv("RUNTOPO_BMC_ADDR"),
		"make virtual BMCs bind to `address`")
	destroy = flag.Bool("destroy", os.Getenv("RUNTOPO_DESTROY") != "",
		"destroy resources created by previous invocation")
)

func main() {
	log.SetFlags(0)
	log.SetPrefix(filepath.Base(os.Args[0]) + ": ")
	if flag.Parse(); flag.NArg() != 1 {
		log.Fatalf("usage: runtopo [options…] topology.dot")
	}
	var topoOpts []topology.Option
	if *autoMgmt {
		topoOpts = append(topoOpts, topology.WithAutoMgmtNetwork)
	}

	keys, err := loadSSHPublicKeys()
	if err != nil {
		log.Fatal(err)
	}
	defaultTunnelIP := net.ParseIP(*tunnelIP)
	if defaultTunnelIP == nil {
		log.Fatalf("cannot parse tunnelip %q", *tunnelIP)
	}

	topo, err := topology.ParseFile(flag.Arg(0), topoOpts...)
	if err != nil {
		log.Fatal(err)
	}

	runnerOpts := []libvirt.RunnerOption{
		libvirt.WithNamePrefix(*namePrefix),
		libvirt.WithPortBase(*portBase),
		libvirt.WithPortGap(*portGap),
		libvirt.WithStoragePool(*storagePool),
		libvirt.WithTunnelIP(defaultTunnelIP),
		libvirt.WithAuthorizedKeys(keys...),
		libvirt.WithConfigFS(os.DirFS(filepath.Dir(flag.Arg(0)))),
	}
	if s := *libvirtURI; s != "" {
		runnerOpts = append(runnerOpts, libvirt.WithConnectionURI(s))
	}
	if s := *macAddrBase; s != "" {
		base, err := net.ParseMAC(s)
		if err != nil {
			log.Fatal(err)
		}
		runnerOpts = append(runnerOpts, libvirt.WithMACAddressBase(base))
	}
	if s := *writeSSHConfig; s != "" {
		fd, err := os.Create(s)
		if err != nil {
			log.Fatal(err)
		}
		defer func() {
			if err := fd.Close(); err != nil {
				log.Printf("writesshconfig: %v", err)
			}
		}()
		runnerOpts = append(runnerOpts, libvirt.WriteSSHConfig(fd))
	}
	if s := *writeBMCConfig; s != "" {
		fd, err := os.Create(s)
		if err != nil {
			log.Fatal(err)
		}
		defer func() {
			if err := fd.Close(); err != nil {
				log.Printf("writebmcconfig: %v", err)
			}
		}()
		runnerOpts = append(runnerOpts, libvirt.WriteBMCConfig(fd))
	}
	if s := *bmcAddr; s != "" {
		runnerOpts = append(runnerOpts, libvirt.WithBMCAddr(s))
	}
	r := libvirt.NewRunner(runnerOpts...)

	ctx := context.TODO()
	if *destroy {
		if err := r.Destroy(ctx, topo); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := r.Run(ctx, topo); err != nil {
		log.Fatal(err)
	}
}

func loadSSHPublicKeys() ([]string, error) {
	home := os.Getenv("HOME")
	if home == "" {
		u, err := user.Current()
		if err != nil {
			return nil, err
		}
		home = u.HomeDir
	}
	dotSSH := filepath.Join(home, ".ssh")
	files, err := filepath.Glob(dotSSH + "/id_*.pub")
	if err != nil {
		return nil, err
	}

	var keys []string
	for _, file := range files {
		p, err := ioutil.ReadFile(file)
		if err != nil {
			return nil, err
		}
		keys = append(keys, string(p))
	}

	return keys, nil
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func atoi(a string) int {
	if i, err := strconv.Atoi(a); err == nil {
		return i
	}
	return 0
}
