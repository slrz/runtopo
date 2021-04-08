package libvirt

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"go4.org/writerutil"
	"golang.org/x/crypto/ssh"
	"slrz.net/runtopo/topology"
)

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

	for hostname, d := range r.devices {
		if d.topoDev.Function() != topology.Host {
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

func proxyJump(c *ssh.Client, addr string, config *ssh.ClientConfig) (cc *ssh.Client, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("proxyJump %s: %w", addr, err)
		}
	}()

	conn, err := c.Dial("tcp", net.JoinHostPort(addr, "22"))
	if err != nil {
		return nil, err
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return ssh.NewClient(sshConn, chans, reqs), nil
}

func runCommand(c *ssh.Client, name string, args ...string) ([]byte, error) {
	var b strings.Builder

	b.WriteString(shellQuote(name))
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteString(shellQuote(a))
	}
	cmd := b.String()

	sess, err := c.NewSession()
	if err != nil {
		return nil, err
	}
	defer sess.Close()

	var stdout bytes.Buffer
	stderr := &writerutil.PrefixSuffixSaver{N: 1024}
	sess.Stdout = &stdout
	sess.Stderr = stderr

	if err := sess.Run(cmd); err != nil {
		if msg := stderr.Bytes(); len(msg) > 0 {
			return nil, fmt.Errorf("runCommand: %w | %s |", err, msg)
		}
		return nil, fmt.Errorf("runCommand: %w", err)
	}

	return stdout.Bytes(), nil
}

func sftpGet(conn *ssh.Client, path string) (content []byte, err error) {
	c, err := sftp.NewClient(conn)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	fd, err := c.Open(path)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	return io.ReadAll(fd)
}

func sftpPutReader(conn *ssh.Client, dstPath string, src io.Reader) (err error) {
	c, err := sftp.NewClient(conn)
	if err != nil {
		return err
	}
	defer c.Close()

	fd, err := c.Create(dstPath)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := fd.Close(); err == nil {
			err = cerr
		}
	}()

	_, err = io.Copy(fd, src)
	return err
}

func sftpPut(conn *ssh.Client, dstPath string, content []byte) (err error) {
	return sftpPutReader(conn, dstPath, bytes.NewReader(content))
}

func sshKeygen(rand io.Reader) (ssh.Signer, []byte, error) {
	_, sk, err := ed25519.GenerateKey(rand)
	if err != nil {
		return nil, nil, err
	}

	signer, err := ssh.NewSignerFromSigner(sk)
	if err != nil {
		return nil, nil, err
	}

	sshPubKey := ssh.MarshalAuthorizedKey(signer.PublicKey())

	return signer, sshPubKey, nil
}

func mustReadFile(path string) []byte {
	p, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return p
}

// ShellQuote returns s in a form suitable to pass it to the shell as an
// argument. Obviously, it works for Bourne-like shells only.  The way this
// works is that first the whole string is enclosed in single quotes. Now the
// only character that needs special handling is the single quote itself.  We
// replace it by '\'' (the outer quotes are part of the replacement) and make
// use of the fact that the shell concatenates adjacent strings.
func shellQuote(s string) string {
	t := strings.Replace(s, "'", `'\''`, -1)
	return "'" + t + "'"
}
