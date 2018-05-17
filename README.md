# nodeos-monitor

`nodeos-monitor` provides failover for EOS nodes and block producers
using Etcd. It can be used for creating highly redundant block
producer architectures, even across data centers.

## Architecture

`nodeos-monitor` alternates a `nodeos` process between two states,
"active" and "standby". Usually, an active node will be an EOS block
producer and a standby node will be a validator node. The `nodeos`
process is switched between states by being killed and restarted with
a new configuration. `nodeos-monitor` tracks the `nodeos` process as a
subprocess.

### Locking

A `nodeos` subprocess is deemed "active" when `nodeos-monitor` can
achieve a distributed lock on an Etcd key. If the lock can't be
attained or if the lock is at some point lost, `nodeos-monitor`
switches `nodeos` to the standby configuration set.

## Usage

### Prerequisites

* Etcd cluster

An Etcd cluster is needed for distributed locking. If the failover
group will cross data center boundaries, the Etcd cluster needs to
span all data centers.

### Etcd resources

[Admin guide](https://coreos.com/etcd/docs/latest/v2/admin_guide.html)

Use this guide for guidance on building out an Etcd cluster.

[Etcd tuning guide](https://coreos.com/etcd/docs/latest/tuning.html)

Etcd clusters must be tuned if they span multiple data centers for
high latencies.

### Installation

On a machine with the Go runtime installed, run

```
$ go get -u github.com/activeeos/nodeos-monitor/cmd/nodeos-monitor
```

In the future, Github releases will be created.

### Configuration

`nodeos-monitor` is configured via command line flags:

```
# nodeos-monitor -h

nodeos-monitor provides failover for EOS nodes

Usage:
  nodeos-monitor [flags]

Flags:
      --active-config-dir string     the directory containing the configs for an active nodeos process (default "/etc/nodeos-active-configs/")
      --debug                        print debug logs
      --etcd-ca string               the Etcd CA to use
      --etcd-cert string             the Etcd client certificate
      --etcd-endpoints stringArray   the endpoints to Etcd (default [http://127.0.0.1:2379])
      --etcd-key string              the Etcd client key
      --failover-group string        the identifier for the group of nodes involved in the failover process (default "eos")
  -h, --help                         help for nodeos-monitor
      --nodeos string                the path to the nodeos binary (default "/opt/eosio/bin/nodeos")
      --nodeos-args stringArray      additional arguments to pass to nodeos
      --standby-config-dir string    the directory containing the configs for a standby nodeos process (default "/etc/nodeos-standby-configs/")
```

Here are the most useful options:

* `active-config-dir`

This is the directory containing a `config.ini` file for the active
`nodeos` process, usually a block producer.

* `standby-config-dir`

This is the directory containing a `config.ini` file for the standby
`nodeos` process, usually a validator node.

* `failover-group`

A failover group is a unique descriptor for the block producer all
nodes are vying to be. This is the key that's used in building the
distributed lock on Etcd. All nodes must be configured with the
identical `failover-group` setting.
