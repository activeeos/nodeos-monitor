# nodeos-monitor

A wrapper for EOS's nodeos process that provides failover and secret
management.

## Architecture

Required services
- Etcd
- Vault

Features
- Switch config file upon leadership election
- Inject vault secrets into config file template
