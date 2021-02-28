# The runtopo Network Simulation Tool

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

## Configuration

## Supported DOT Attributes

### Node Attributes

### Edge Attributes

## Defaults
