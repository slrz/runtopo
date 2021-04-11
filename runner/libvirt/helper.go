package libvirt

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"inet.af/netaddr"
	"libvirt.org/libvirt-go"
	"slrz.net/runtopo/topology"
)

type eofReader struct{}

func (eofReader) Read([]byte) (int, error) {
	return 0, io.EOF
}

// RandomString generates a printable random string of length n using a
// cryptographically-secure RNG.
func randomString(n int) string {
	scratch := make([]byte, (n+3)/4*3)
	if _, err := rand.Read(scratch); err != nil {
		panic(err)
	}

	return base64.URLEncoding.EncodeToString(scratch)[:n]
}

// ValidateDomainXML validates the provided XML against the libvirt domain
// schema.
func validateDomainXML(xmlBytes []byte) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("validateDomainXML: %w", err)
		}
	}()

	// Write xmlBytes to a temp file as older versions of virt-xml-validate
	// can't read from stdin (and even recent ones would just do the temp
	// file thing internally).
	fd, err := ioutil.TempFile("", "runtopo-domxml.")
	if err != nil {
		return err
	}
	defer os.Remove(fd.Name())

	if _, err := fd.Write(xmlBytes); err != nil {
		return err
	}
	if err := fd.Close(); err != nil {
		return err
	}

	var stderr bytes.Buffer
	cmd := exec.Command("virt-xml-validate", fd.Name(), "domain")
	cmd.Stdin = bytes.NewReader(xmlBytes)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%v (%s)", err, stderr.Bytes())
	}

	return nil
}

func createVolume(ctx context.Context, path, backingStore string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("createVolume: %w", err)
		}
	}()

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "qemu-img", "create", "-fqcow2",
		"-Fqcow2", "-b"+backingStore, path)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%v (%s)", err, stderr.Bytes())
	}

	return nil
}

func mustParseMAC(s string) net.HardwareAddr {
	hw, err := net.ParseMAC(s)
	if err != nil {
		panic("mustParseMAC: " + err.Error())
	}
	return hw
}

func fetchImageContentLength(ctx context.Context, imageURL string) (n int64, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("fetchImageContentLength: %w (url: %s)",
				err, imageURL)
		}
	}()
	req, err := http.NewRequestWithContext(ctx, "HEAD", imageURL, nil)
	if err != nil {
		return 0, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if !statusOK(resp) {
		return 0, fmt.Errorf("status %s", resp.Status)
	}

	return strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
}

func fetchImageToFile(ctx context.Context, outFile, fromURL string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("fetchImageToFile: %w (url: %s)", err, fromURL)
		}
	}()

	fd, err := ioutil.TempFile(filepath.Split(outFile))
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(fd.Name())
		}
	}()

	if err := fetchImage(ctx, fd, fromURL); err != nil {
		fd.Close()
		return err
	}
	if err := fd.Close(); err != nil {
		return err
	}

	return os.Rename(fd.Name(), outFile)
}

func fetchImage(ctx context.Context, w io.Writer, url string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("fetchImage: %w (url: %s)", err, url)
		}
	}()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if !statusOK(resp) {
		return fmt.Errorf("status %s", resp.Status)
	}

	_, err = io.Copy(w, resp.Body)
	return err
}

func statusOK(r *http.Response) bool {
	return 200 <= r.StatusCode && r.StatusCode < 300
}

func macAddrFromUint64(x uint64) net.HardwareAddr {
	if x&((1<<48)-1) != x {
		panic(fmt.Sprintf("invalid EUI-48: %x", x))
	}
	var a [8]byte
	binary.BigEndian.PutUint64(a[:], x)

	return net.HardwareAddr(a[2:])
}

// Compare s and t using Dave Koelle's Alphanum algorithm for natural sorting.
func natCompare(s, t string) int {
	nextChunk := func(s string) string {
		var p []byte
		c, s := s[0], s[1:]
		p = append(p, c)

		if isASCIIDigit(rune(c)) {
			for len(s) > 0 {
				c := s[0]
				if !isASCIIDigit(rune(c)) {
					break
				}
				p = append(p, c)
				s = s[1:]
			}
			return string(p)
		}
		for len(s) > 0 {
			c := s[0]
			if isASCIIDigit(rune(c)) {
				break
			}
			p = append(p, c)
			s = s[1:]
		}

		return string(p)
	}

	for len(s) > 0 && len(t) > 0 {
		cs := nextChunk(s)
		s = s[len(cs):]
		ct := nextChunk(t)
		t = t[len(ct):]

		if isASCIIDigit(rune(cs[0])) && isASCIIDigit(rune(ct[0])) {
			is, it := mustAtoi(cs), mustAtoi(ct)
			if is > it {
				return 1
			}
			if is < it {
				return -1
			}
		}
		if cmp := strings.Compare(cs, ct); cmp != 0 {
			return cmp
		}
	}

	return len(s) - len(t)
}

func mustAtoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return i
}

func isASCIIDigit(c rune) bool {
	return '0' <= c && c <= '9'
}

func hasFunction(d *device, fs ...topology.DeviceFunction) bool {
	return topology.HasFunction(&d.topoDev, fs...)
}

// Returns whether d defaults to Cumulus Linux.
func hasCumulusFunction(d *device) bool {
	return hasFunction(d,
		topology.OOBSwitch,
		topology.Exit,
		topology.SuperSpine,
		topology.Spine,
		topology.Leaf,
		topology.TOR,
	)
}

// Waits until d received a DHCP lease from a libvirt network and return its
// address.
func waitForLease(ctx context.Context, d *libvirt.Domain) (ip netaddr.IP, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("waitForLease: %w", err)
		}
	}()
	var intf libvirt.DomainInterface
	for {
		xs, err1 := d.ListAllInterfaceAddresses(
			libvirt.DOMAIN_INTERFACE_ADDRESSES_SRC_LEASE,
		)
		if err1 != nil {
			err = err1
			break
		}
		if len(xs) > 0 {
			intf = xs[0]
			break
		}

		select {
		case <-ctx.Done():
			return netaddr.IP{}, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if err != nil {
		return netaddr.IP{}, err
	}
	if len(intf.Addrs) == 0 {
		return netaddr.IP{}, fmt.Errorf(
			"interface %s: no addresses (hwaddr=%s)",
			intf.Name, intf.Hwaddr)
	}
	return netaddr.ParseIP(intf.Addrs[0].Addr)
}

func writeTempFile(dir, prefix string, p []byte) (file string, err error) {
	fd, err := ioutil.TempFile(dir, prefix)
	if err != nil {
		return "", err
	}
	defer func() {
		if cerr := fd.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			err = fmt.Errorf("writeTempFile: %w", err)
		}
	}()

	if _, err := fd.Write(p); err != nil {
		return "", err
	}

	return fd.Name(), nil
}
