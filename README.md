# The runtopo Network Simulation Tool
[![Go Reference](https://pkg.go.dev/badge/slrz.net/runtopo.svg)](https://pkg.go.dev/slrz.net/runtopo)

Runtopo simulates a network topology described using the [DOT graph description
language](https://www.graphviz.org/doc/info/lang.html). Network elements and
hosts are simulated using virtual machines, wired up to represent the provided
topology file.

It is similar to (and somewhat compatible with) Cumulus Networks'
[topology\_converter](https://gitlab.com/cumulus-consulting/tools/topology_converter)
but doesn't rely on Vagrant. Instead, runtopo interacts directly with libvirt.
This makes it more flexible (at least potentially) and allows for a faithful
simulation of additional scenarios that are either hard or infeasible to
realize while accomodating Vagrant's requirements.


## What runtopo Is Not

If you want to run a simulated topology on your laptop (possibly running a
non-Linux OS like Windows or macOS) you should stay with topology\_converter.
Runtopo currently only supports libvirt using the QEMU driver. There are no
plans for supporting VirtualBox or similar. In addition, topology\_converter is
easier to get started with whereas runtopo requires a more extensive initial
setup process.


## Installation

```
go get [-u] slrz.net/runtopo
```

## Configuration

Coming soon.

## Supported DOT Attributes

The following attributes are supported on nodes and edges, respectively. If not
supplied, a (possibly configurable) default is used.

### Node Attributes
* os -- sets the operating system image to use for the device. Should be a URL
  referring to a QCOW2 image. **NOTE:** this differs from topology\_converter
  which wants a Vagrant box specified here.
* config -- a provisioning script executed in the context of the device
* cpu -- number of VCPUs to assign to device
* memory -- device memory size in MiB
* tunnelip -- IP address for libvirt UDP tunnels associated with this device
* mgmt\_ip -- creates DHCP reservation when AutoMgmtNetwork is enabled
* no\_mgmt -- do not create management interface even when AutoMgmtNetwork is enabled
* function -- one of [oob-server, oob-switch, exit, superspine, spine, leaf,
  tor, host] or *fake* to not simulate the device at all but still make links
  appear as up to the remote side

### Edge Attributes
* left\_mac/right\_mac -- explicitly specify MAC address for interface
* left\_pxe/right\_pxe -- configure interface for PXE boot

## Defaults

Coming soon.
