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
	"strings"
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

	// Upload device configuration
	for hostname := range r.devices {
		configDir := filepath.Join("testdata/configs", hostname)
		files, err := os.ReadDir(configDir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			t.Fatal(err)
		}
		if len(files) == 0 {
			continue
		}
		reload, err := os.ReadFile(filepath.Join("testdata/reload", hostname))
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			reload = nil
		}
		err = withBackoff(nretries, func() error {
			c, err := proxyJump(oob, hostname, sshConfig)
			if err != nil {
				return err
			}
			defer c.Close()

			for _, f := range files {
				// Slashes are represented as "--" in the
				// testdata file names, so reconstruct the
				// original path.
				dst := "/" + strings.Replace(f.Name(), "--", "/", -1)
				src := filepath.Join(configDir, f.Name())
				err := sftpPut(c, dst, mustReadFile(src))
				if err != nil {
					return err
				}
			}
			if reload == nil {
				return nil
			}
			if err := sftpPut(c, "/tmp/runtopo-reload", reload); err != nil {
				return err
			}
			_, err = runCommand(c, "/bin/sh", "/tmp/runtopo-reload")
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

func mustReadFile(path string) []byte {
	p, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return p
}
