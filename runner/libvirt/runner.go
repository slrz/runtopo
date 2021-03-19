package libvirt

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"libvirt.org/libvirt-go"
	"slrz.net/runtopo/topology"
)

// Runner implements the topology.Runner interface using libvirt/qemu.
type Runner struct {
	conn         *libvirt.Connect
	devices      map[string]*device
	domains      map[string]*libvirt.Domain
	baseImages   map[string]*libvirt.StorageVol
	sshConfigOut io.Writer

	// fields below are immutable after initialization
	uri            string // libvirt connection URI
	namePrefix     string
	tunnelIP       net.IP
	macBase        net.HardwareAddr
	portBase       int
	portGap        int
	storagePool    string
	imageDir       string
	authorizedKeys []string
}

// A RunnerOption may be passed to NewRunner to customize the Runner's
// behaviour.
type RunnerOption func(*Runner)

// WithConnectionURI sets the connection URI used to connect to libvirtd.
// Defaults to "qemu:///system".
func WithConnectionURI(uri string) RunnerOption {
	return func(r *Runner) {
		r.uri = uri
	}
}

// WithNamePrefix configures the prefix to use when naming resources like guest
// domains. The default is "runtopo-".
func WithNamePrefix(prefix string) RunnerOption {
	return func(r *Runner) {
		r.namePrefix = prefix
	}
}

// WithTunnelIP sets the default IP address for libvirt UDP tunnels. This is
// used only for devices that do not have an explicit address configured
// (tunnelip node attribute).
func WithTunnelIP(ip net.IP) RunnerOption {
	return func(r *Runner) {
		r.tunnelIP = ip
	}
}

// WithMACAddressBase determines the starting address for automatically
// assigned MAC addresses. Explicitly configured MAC addresses
// (left_mac/right_mac edge attributes) are unaffected by this option.
func WithMACAddressBase(mac net.HardwareAddr) RunnerOption {
	return func(r *Runner) {
		r.macBase = mac
	}
}

// WithPortBase specifies the starting port for allocating UDP tunnel ports.
func WithPortBase(port int) RunnerOption {
	return func(r *Runner) {
		r.portBase = port
	}
}

// WithPortGap sets the gap left between local and remote port. It limits
// the maximum number of connections supported in a topology.
func WithPortGap(delta int) RunnerOption {
	return func(r *Runner) {
		r.portGap = delta
	}
}

// WithStoragePool sets the libvirt storage pool where we create volumes.
func WithStoragePool(pool string) RunnerOption {
	return func(r *Runner) {
		r.storagePool = pool
	}
}

// WithImageDirectory sets the target directory where to create volumes. This
// causes the runner to create volumes using direct file system operations
// instead of libvirt. Prefer using WithStoragePool.
func WithImageDirectory(dir string) RunnerOption {
	return func(r *Runner) {
		r.imageDir = dir
	}
}

// WithAuthorizedKeys adds the provided SSH public keys to authorized_keys for
// all started VMs.
func WithAuthorizedKeys(keys ...string) RunnerOption {
	return func(r *Runner) {
		r.authorizedKeys = keys
	}
}

// WriteSSHConfig configures the Runner to write an OpenSSH client
// configuration file to w. See ssh_config(5) for a description of its format.
func WriteSSHConfig(w io.Writer) RunnerOption {
	return func(r *Runner) {
		r.sshConfigOut = w
	}
}

// NewRunner constructs a runner configured with the specified options.
func NewRunner(opts ...RunnerOption) *Runner {
	r := &Runner{
		uri:        "qemu:///system",
		namePrefix: "runtopo-",
		tunnelIP:   net.IPv4(127, 0, 0, 1),

		// BUG(ls): The default MAC address range matches the one used
		// by topology_converter. It belongs to Cumulus though and we
		// probably shouldn't use it without asking them.
		macBase:     mustParseMAC("44:38:39:00:00:00"),
		portBase:    1e4,
		portGap:     1e3,
		storagePool: "default",
		devices:     make(map[string]*device),
		domains:     make(map[string]*libvirt.Domain),
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Run starts up the topology described by t.
func (r *Runner) Run(ctx context.Context, t *topology.T) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("libvirt.(*Runner).Run: %w", err)
		}
	}()

	if n := len(r.macBase); n != 6 {
		return fmt.Errorf("got base MAC of len %d, want len 6", n)
	}
	if err := r.buildInventory(t); err != nil {
		return err
	}

	c, err := libvirt.NewConnect(r.uri)
	if err != nil {
		return err
	}
	r.conn = c
	if err := r.downloadBaseImages(ctx, t); err != nil {
		return err
	}
	if err := r.createVolumes(ctx, t); err != nil {
		return err
	}

	if err := r.defineDomains(ctx, t); err != nil {
		return err
	}
	if err := r.customizeDomains(ctx, t); err != nil {
		return err
	}
	if err := r.startDomains(ctx, t); err != nil {
		return err
	}

	if r.sshConfigOut != nil {
		// Caller asked us to write out an ssh_config.
		if err := r.writeSSHConfig(ctx, t); err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) buildInventory(t *topology.T) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("buildInventory: %w", err)
		}
	}()

	var macInt uint64
	for _, b := range r.macBase {
		macInt = macInt<<8 | uint64(b)
	}

	allocateMAC := func() net.HardwareAddr {
		mac := macAddrFromUint64(macInt)
		macInt++
		return mac
	}

	for _, topoDev := range t.Devices() {
		if topoDev.Function() == topology.FunctionFake {
			continue
		}

		tunnelIP := r.tunnelIP
		if s := topoDev.Attr("tunnelip"); s != "" {
			if tunnelIP = net.ParseIP(s); tunnelIP == nil {
				return fmt.Errorf(
					"device %s: cannot parse tunnelip %q",
					topoDev.Name, s)
			}
		}
		u, err := url.Parse(topoDev.OSImage())
		if err != nil {
			return err
		}
		base := filepath.Join(r.imageDir, path.Base(u.Path))
		r.devices[topoDev.Name] = &device{
			name:      r.namePrefix + topoDev.Name,
			tunnelIP:  tunnelIP,
			image:     filepath.Join(r.imageDir, r.namePrefix+topoDev.Name),
			baseImage: base,
			pool:      r.storagePool,
			topoDev:   topoDev,
		}
	}
	nextPort := uint(r.portBase)
	for _, l := range t.Links() {
		if from := r.devices[l.From]; from != nil {
			mac, hasMAC := l.FromMAC()
			if !hasMAC {
				mac = allocateMAC()
			}
			if (l.From == "oob-mgmt-server" || l.From == "oob-mgmt-switch") &&
				l.To == "" {
				// XXX
				from.interfaces = append(from.interfaces, iface{
					name:    l.FromPort,
					mac:     mac,
					network: "default",
				})
				continue
			}
			from.interfaces = append(from.interfaces, iface{
				name:      l.FromPort,
				mac:       mac,
				port:      nextPort,
				localPort: nextPort + uint(r.portGap),
			})
		}
		if to := r.devices[l.To]; to != nil {
			mac, hasMAC := l.ToMAC()
			if !hasMAC {
				mac = allocateMAC()
			}
			to.interfaces = append(to.interfaces, iface{
				name:      l.ToPort,
				mac:       mac,
				port:      nextPort + uint(r.portGap),
				localPort: nextPort,
			})

		}
		nextPort++
	}

	for _, d := range r.devices {
		sort.Slice(d.interfaces, func(i, j int) bool {
			di, dj := d.interfaces[i], d.interfaces[j]
			if di.name == "eth0" && dj.name != "eth0" {
				return true
			}
			return natCompare(di.name, dj.name) < 0
		})
	}
	return nil
}

func (r *Runner) downloadBaseImages(ctx context.Context, t *topology.T) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("downloadBaseImages: %w", err)
		}
	}()
	pool, err := r.conn.LookupStoragePoolByName(r.storagePool)
	if err != nil {
		return err
	}
	defer pool.Free()

	wantImages := make(map[string]struct{})
	haveImages := make(map[string]*libvirt.StorageVol)
	for _, d := range r.devices {
		u, err := url.Parse(d.topoDev.OSImage())
		if err != nil {
			return err
		}
		vol, err := pool.LookupStorageVolByName(path.Base(u.Path))
		if err == nil {
			// skip over already present volumes
			haveImages[d.topoDev.OSImage()] = vol
			continue
		}
		wantImages[d.topoDev.OSImage()] = struct{}{}
	}

	type result struct {
		vol *libvirt.StorageVol
		url string
		err error
	}
	ch := make(chan result)
	fetchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	numStarted := 0
	for sourceURL := range wantImages {
		sourceURL := sourceURL
		go func() {
			vol, err := createVolumeFromURL(fetchCtx, r.conn, pool, sourceURL)
			if err != nil {
				ch <- result{err: err, url: sourceURL}
				return
			}
			ch <- result{vol: vol, url: sourceURL}

		}()
		numStarted++
	}

	for i := 0; i < numStarted; i++ {
		res := <-ch
		if res.err == nil {
			haveImages[res.url] = res.vol
			continue
		}
		if res.err != nil {
			cancel() // tell other goroutines to shut down
			if err == nil {
				err = res.err
			}
		}
	}
	if err != nil {
		for _, v := range haveImages {
			v.Free()
		}
		return err
	}
	r.baseImages = haveImages
	return nil
}

func (r *Runner) downloadBaseImagesDirect(ctx context.Context, t *topology.T) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("downloadBaseImages: %w", err)
		}
	}()
	wantImages := make(map[string]struct{})
	for _, d := range r.devices {
		wantImages[d.topoDev.OSImage()] = struct{}{}
	}

	ch := make(chan error)
	numToFetch := 0
	for sourceURL := range wantImages {
		sourceURL := sourceURL
		u, err := url.Parse(sourceURL)
		if err != nil {
			return err
		}
		ofile := filepath.Join(r.imageDir, path.Base(u.Path))
		_, err = os.Stat(ofile)
		if err == nil {
			continue
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		numToFetch++
		go func() {
			ch <- fetchImageToFile(ctx, ofile, sourceURL)
		}()
	}
	for i := 0; i < numToFetch; i++ {
		if fetchErr := <-ch; fetchErr != nil && err == nil {
			err = fetchErr
		}
	}

	return err
}

func (r *Runner) createVolumes(ctx context.Context, t *topology.T) (err error) {
	var created []*libvirt.StorageVol
	defer func() {
		if err != nil {
			for _, vol := range created {
				vol.Delete(0)
				vol.Free()

			}
			err = fmt.Errorf("createVolumes: %w", err)
		}
	}()
	pool, err := r.conn.LookupStoragePoolByName(r.storagePool)
	if err != nil {
		return err
	}
	defer pool.Free()

	for _, d := range r.devices {
		base := r.baseImages[d.topoDev.OSImage()]
		if base == nil {
			// we should've failed earlier already
			panic("unexpected missing base image: " +
				d.topoDev.OSImage())
		}
		baseInfo, err := base.GetInfo()
		if err != nil {
			return fmt.Errorf("get-info: %w (bvol: %s)",
				err, d.topoDev.OSImage())
		}

		xmlVol := newVolume(d.name, int64(baseInfo.Capacity))
		backing, err := newBackingStoreFromVol(base)
		if err != nil {
			return err
		}
		xmlVol.BackingStore = backing
		xmlStr, err := xmlVol.Marshal()
		if err != nil {
			return err
		}
		vol, err := pool.StorageVolCreateXML(xmlStr, 0)
		if err != nil {
			return fmt.Errorf("vol-create: %w", err)
		}
		created = append(created, vol)
		d.pool = r.storagePool
		d.volume = vol
	}

	return nil
}

func (r *Runner) createVolumesDirect(ctx context.Context, t *topology.T) (err error) {
	var created []string
	defer func() {
		if err != nil {
			for _, file := range created {
				os.Remove(file)
			}
			err = fmt.Errorf("createDiffImages: %w", err)
		}
	}()
	for _, d := range r.devices {
		u, err := url.Parse(d.topoDev.OSImage())
		if err != nil {
			return err
		}
		base := filepath.Join(r.imageDir, path.Base(u.Path))
		diff := filepath.Join(r.imageDir, d.name)
		if err := createVolume(ctx, diff, base); err != nil {
			return err
		}
		created = append(created, diff)
	}

	return nil
}

func (r *Runner) defineDomains(ctx context.Context, t *topology.T) (err error) {
	var defined []*libvirt.Domain
	defer func() {
		if err != nil {
			for _, dom := range defined {
				dom.Undefine()
				dom.Free()
			}
			err = fmt.Errorf("defineDomains: %w", err)
		}
	}()
	tmpl, err := template.New("").
		Funcs(templateFuncs).
		Parse(domainTemplateText)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	for _, d := range r.devices {
		if err := tmpl.Execute(&buf, d.templateArgs()); err != nil {
			return fmt.Errorf("domain %s: %w", d.name, err)
		}
		domXML := buf.String()
		buf.Reset()
		dom, err := r.conn.DomainDefineXMLFlags(
			domXML, libvirt.DOMAIN_DEFINE_VALIDATE)
		if err != nil {
			return fmt.Errorf("define domain %s: %w", d.name, err)
		}
		defined = append(defined, dom)
		r.domains[d.name] = dom
	}
	return nil
}

func (r *Runner) customizeDomains(ctx context.Context, t *topology.T) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("customizeDomains: %w", err)
		}
	}()

	var buf bytes.Buffer
	ch := make(chan error)
	numStarted := 0
	customizeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	for _, d := range r.devices {
		user := "root"
		if isCumulusFunction(d.topoDev.Function()) {
			user = "cumulus"
		}
		for _, k := range r.authorizedKeys {
			fmt.Fprintf(&buf, "ssh-inject %s:string:%s\n", user, k)
		}
		if d.topoDev.Function() == topology.FunctionOOBServer {
			hosts := gatherHosts(ctx, r, t)
			for _, h := range hosts {
				fmt.Fprintf(&buf, "append-line /etc/hosts:%s %s\n",
					h.ip, h.name)
			}
			dnsmasqHosts := generateDnsmasqHostsFile(hosts)
			fmt.Fprintf(&buf, "write /etc/dnsmasq.hostsfile:%s\n",
				bytes.Replace(dnsmasqHosts, []byte("\n"),
					[]byte("\\\n"), -1))
		}
		extra := strings.NewReader(buf.String())
		buf.Reset()
		d := d
		go func() {
			ch <- customizeDomain(customizeCtx, r.uri, d, extra)
		}()
		numStarted++
	}

	for i := 0; i < numStarted; i++ {
		res := <-ch
		if res != nil {
			cancel() // tell other goroutines to shut down
			if err == nil {
				err = res
			}
		}
	}
	return err
}

func (r *Runner) startDomains(ctx context.Context, t *topology.T) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("startDomains: %w", err)
		}
	}()
	ds := t.Devices()
	sort.Slice(ds, func(i, j int) bool {
		return ds[i].Function() < ds[j].Function()
	})

	var started []*libvirt.Domain
	defer func() {
		if err != nil {
			for _, d := range started {
				d.Destroy()
			}
		}
	}()
	for _, d := range ds {
		dom := r.domains[r.namePrefix+d.Name]
		if err := dom.Create(); err != nil {
			return fmt.Errorf("domain %s: %w",
				r.namePrefix+d.Name, err)
		}
		started = append(started, dom)
	}

	return nil
}

// WriteSSHConfig genererates an OpenSSH client config and writes it to r.sshConfigOut.
func (r *Runner) writeSSHConfig(ctx context.Context, t *topology.T) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("writeSSHConfig: %w", err)
		}
	}()

	// Retrieve the mgmt server's DHCP lease as we're going to use
	// it as a jump host.
	ip, err := waitForLease(ctx, r.domains[r.namePrefix+"oob-mgmt-server"])
	if err != nil {
		return err
	}

	w := bufio.NewWriter(r.sshConfigOut)

	fmt.Fprintf(w, `Host oob-mgmt-server
  Hostname %s
  User root
  UserKnownHostsFile /dev/null
  StrictHostKeyChecking no
`, ip)

	for _, d := range t.Devices() {
		if d.Function() == topology.FunctionOOBServer ||
			d.Function() == topology.FunctionOOBSwitch {
			continue
		}
		user := "root"
		if isCumulusFunction(d.Function()) {
			user = "cumulus"
		}
		fmt.Fprintf(w, `Host %s
  User %s
  ProxyJump oob-mgmt-server
  UserKnownHostsFile /dev/null
  StrictHostKeyChecking no
`, d.Name, user)

	}

	return w.Flush()
}

// internal representation for a device
type device struct {
	name       string
	tunnelIP   net.IP
	interfaces []iface
	volume     *libvirt.StorageVol
	pool       string
	image      string
	baseImage  string
	topoDev    topology.Device
}

func (d *device) templateArgs() *domainTemplateArgs {
	args := &domainTemplateArgs{
		Name:      d.name,
		VCPUs:     d.topoDev.VCPUs(),
		Memory:    d.topoDev.Memory() >> 10, // libvirt wants KiB
		Pool:      d.pool,
		Image:     d.image,
		BaseImage: d.baseImage,
	}
	for _, intf := range d.interfaces {
		typ := "udp"
		netSrc, udpSrc := intf.network, udpSource{
			// BUG(ls): We need the remote device's tunnel IP to
			// allow a single topology to span multiple hosts. Fix
			// it by stashing away the remote IP during
			// buildInventory.
			Address:      d.tunnelIP.String(),
			Port:         intf.port,
			LocalAddress: d.tunnelIP.String(),
			LocalPort:    intf.localPort,
		}
		if intf.network != "" {
			typ = "network"
		}
		args.Interfaces = append(args.Interfaces, domainInterface{
			Type:          typ,
			MACAddr:       intf.mac.String(),
			TargetDev:     intf.name,
			Model:         "virtio",
			NetworkSource: netSrc,
			UDPSource:     udpSrc,
		})
	}

	return args
}

// internal representation for an interface
type iface struct {
	name      string
	mac       net.HardwareAddr
	network   string
	port      uint
	localPort uint
}
