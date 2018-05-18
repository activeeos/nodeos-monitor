.PHONY: nodeos-monitor test dev_etcd

nodeos-monitor:
	go install github.com/activeeos/nodeos-monitor/cmd/nodeos-monitor

test:
	go test -v github.com/activeeos/nodeos-monitor/pkg/... github.com/activeeos/nodeos-monitor/cmd/...

test_race:
	go test -v -race github.com/activeeos/nodeos-monitor/pkg/... github.com/activeeos/nodeos-monitor/cmd/...

dev_etcd:
	docker run -it --rm --name nodeos-etcd -p 22379:2379 quay.io/coreos/etcd:v3.3 \
		/usr/local/bin/etcd --listen-client-urls 'http://0.0.0.0:2379'  \
		--advertise-client-urls 'http://0.0.0.0:2379'
