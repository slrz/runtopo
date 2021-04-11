package libvirt

// SSH helpers used in tests.

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/pkg/sftp"
	"go4.org/writerutil"
	"golang.org/x/crypto/ssh"
)

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
