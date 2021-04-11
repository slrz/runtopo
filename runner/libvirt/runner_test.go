package libvirt

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"slrz.net/runtopo/topology"
)

type ptmDetail struct {
	Port             string `json:"port"`
	Status           string `json:"cbl status"`
	ActualNeighbor   string `json:"act nbr"`
	ExpectedNeighbor string `json:"exp nbr"`
}

func TestRuntopo(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	topo, err := topology.ParseFile(
		"testdata/leafspine-with-servers.dot",
		topology.WithAutoMgmtNetwork)
	if err != nil {
		t.Fatal(err)
	}

	signer, pubKey, err := sshKeygen(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	r := NewRunner(
		WithNamePrefix(t.Name()+"-"),
		WithAuthorizedKeys(string(pubKey)),
		WithConfigFS(os.DirFS("testdata")),
	)

	ctx := context.Background()
	var cancel context.CancelFunc
	if deadline, ok := t.Deadline(); ok {
		ctx, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}

	if err := r.Run(ctx, topo); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Destroy(ctx, topo); err != nil {
			t.Error(err)
		}
	}()

	mgmtIP, err := waitForLease(ctx, r.domains[r.namePrefix+"oob-mgmt-server"])
	if err != nil {
		t.Fatal(err)
	}
	sshConfig := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	const nretries = 10
	var oob *ssh.Client
	err = withBackoff(nretries, func() error {
		c, err := ssh.Dial("tcp",
			net.JoinHostPort(mgmtIP.String(), "22"),
			sshConfig)
		if err != nil {
			return err
		}
		oob = c
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer oob.Close()

	// Upload configuration for network devices (frr.conf and interfaces)
	for hostname := range r.devices {
		var files [2][]byte
		sources := []string{
			filepath.Join("testdata/configs/interfaces", hostname),
			filepath.Join("testdata/configs/frr", hostname),
		}

		for i, src := range sources {
			p, err := os.ReadFile(src)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				t.Fatal(err)
			}
			files[i] = p
		}

		if files[0] == nil && files[1] == nil {
			continue
		}

		interfaces, frrConf := files[0], files[1]
		err = withBackoff(nretries, func() error {
			c, err := proxyJump(oob, hostname, sshConfig)
			if err != nil {
				return err
			}
			defer c.Close()

			if len(interfaces) > 0 {
				err := sftpPut(c, "/etc/network/interfaces",
					interfaces)
				if err != nil {
					return err
				}
			}
			if len(frrConf) > 0 {
				return sftpPut(c, "/etc/frr/frr.conf", frrConf)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}

		err = withBackoff(nretries, func() error {
			c, err := proxyJump(oob, hostname, sshConfig)
			if err != nil {
				return err
			}
			defer c.Close()
			commands := [][]string{
				{"sed", "-i", "s/^bgpd=no/bgpd=yes/", "/etc/frr/daemons"},
				{"ifreload", "-a"},
				{"systemctl", "restart", "frr.service"},
			}
			for _, argv := range commands {
				_, err := runCommand(c, argv[0], argv[1:]...)
				if err != nil {
					return err
				}
			}
			t.Logf("=== %s ===", hostname)
			p, err := runCommand(c, "net", "show", "int")
			t.Logf("%s\n===", p)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	t.Run("config-nodeattr", func(t *testing.T) {
		for hostname, d := range r.devices {
			if !hasFunction(d, topology.Host) {
				continue
			}
			var fileData []byte
			err := withBackoff(nretries, func() error {
				c, err := proxyJump(oob, hostname, sshConfig)
				if err != nil {
					return err
				}
				defer c.Close()

				p, err := sftpGet(c, "/kilroywashere")
				if err != nil {
					return err
				}

				fileData = p
				return nil
			})
			if err != nil {
				t.Errorf("%s: %v (giving up after %d retries)",
					hostname, err, nretries)
				continue
			}
			if !bytes.Equal(fileData, []byte("abcdef\n")) {
				t.Errorf("%s: unexpected file content: got %q, want %q",
					hostname, fileData, "abcdef\n")
			}
		}
	})
	t.Run("ptm-topology", func(t *testing.T) {
		for hostname, d := range r.devices {
			if !hasFunction(d, topology.Spine, topology.Leaf) {
				continue
			}
			err := withBackoff(nretries, func() error {
				c, err := proxyJump(oob, hostname, sshConfig)
				if err != nil {
					return err
				}
				defer c.Close()

				p, err := runCommand(c, "ptmctl", "--json", "--detail")
				if err != nil {
					return err
				}
				// Ptmctl gives us a JSON object with numeric
				// string indices: {"0": {}, "1": {}, ...}.
				ptm := make(map[string]*ptmDetail)
				if err := json.Unmarshal(p, &ptm); err != nil {
					return err
				}
				for _, v := range ptm {
					if v.Status != "pass" {
						return fmt.Errorf("%s: got %s, want %s",
							v.Port, v.ActualNeighbor, v.ExpectedNeighbor)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatalf("%s: %v", hostname, err)
			}
		}
	})
}

func withBackoff(attempts int, f func() error) (err error) {
	for i := 0; i < attempts; i++ {
		if err = f(); err == nil {
			return nil
		}
		backoff(i)
	}
	return err
}

func backoff(attempt int) {
	const (
		baseDelay = 1 * time.Second
		maxDelay  = 10 * time.Second
	)
	// Don't use outside tests (ignores overflow, lacks randomization, …).
	d := time.Duration(minInt64(
		(int64(1)<<attempt)*int64(baseDelay),
		int64(maxDelay),
	))
	time.Sleep(d)
}

func minInt64(a, b int64) int64 {
	if b < a {
		return b
	}
	return a
}
