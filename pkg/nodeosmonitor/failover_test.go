package nodeosmonitor_test

import (
	"context"
	"testing"
	"time"

	"code.cloudfoundry.org/clock/fakeclock"
	"github.com/activeeos/nodeos-monitor/pkg/nodeosmonitor"
	"github.com/coreos/etcd/clientv3"
	"github.com/google/uuid"
)

func failoverManager(ctx context.Context, t *testing.T) (*nodeosmonitor.FailoverConfig,
	*nodeosmonitor.FailoverManager) {
	clock := fakeclock.NewFakeClock(time.Now())
	client := nodeosmonitor.GetEtcdClient(t)
	id := uuid.New().String()
	key := uuid.New().String()
	activeProcess := &mockMonitorable{}
	standbyProcess := &mockMonitorable{}
	leaseManager := nodeosmonitor.NewEtcdLeaseManager(clock, client.Lease)
	go leaseManager.Start(ctx)

	conf := &nodeosmonitor.FailoverConfig{
		ID:             id,
		EtcdKey:        key,
		Clock:          clock,
		WatcherClient:  client.Watcher,
		KVClient:       client.KV,
		ActiveProcess:  activeProcess,
		StandbyProcess: standbyProcess,
		LeaseManager:   leaseManager,
	}

	return conf, nodeosmonitor.NewFailoverManager(conf)
}

func TestFailoverManagerActivateImmediatelySuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	conf, failoverManager := failoverManager(ctx, t)

	failoverManager.Start(ctx)
	if err := failoverManager.TryActivate(ctx); err != nil {
		t.Fatal(err)
	}

	if !conf.ActiveProcess.(*mockMonitorable).isActivated() {
		t.Fatal("process should be activated")
	}
}

func TestFailoverManagerActivateImmediatelyFailed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	conf, failoverManager := failoverManager(ctx, t)

	// Set a lease on the key before we use it so that it's
	// inaccessible.
	response, err := nodeosmonitor.GetEtcdClient(t).Lease.Grant(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conf.KVClient.Put(ctx, conf.EtcdKey, "woo",
		clientv3.WithLease(response.ID)); err != nil {
		t.Fatal(err)
	}

	failoverManager.Start(ctx)
	if err := failoverManager.TryActivate(ctx); err != nil {
		t.Fatal(err)
	}

	if !conf.StandbyProcess.(*mockMonitorable).isActivated() {
		t.Fatal("process should be activated")
	}
}

func TestFailoverManagerActivatePeriodicallySuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	conf, failoverManager := failoverManager(ctx, t)

	// TODO: figure out how to remove time dependency from here, maybe
	// by watching the Etcd key.

	failoverManager.Start(ctx)
	time.Sleep(time.Second)

	fc := conf.Clock.(*fakeclock.FakeClock)
	fc.WaitForWatcherAndIncrement(10 * time.Second)
	time.Sleep(time.Second)

	if !conf.ActiveProcess.(*mockMonitorable).isActivated() {
		t.Fatal("process should be activated")
	}
}

func TestFailoverManagerActivatePeriodicallyFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	conf, failoverManager := failoverManager(ctx, t)

	// Set a lease on the key before we use it so that it's
	// inaccessible.
	response, err := nodeosmonitor.GetEtcdClient(t).Lease.Grant(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conf.KVClient.Put(ctx, conf.EtcdKey, "woo",
		clientv3.WithLease(response.ID)); err != nil {
		t.Fatal(err)
	}

	// TODO: figure out how to remove time dependency from here, maybe
	// by watching the Etcd key.

	failoverManager.Start(ctx)
	time.Sleep(time.Second)

	fc := conf.Clock.(*fakeclock.FakeClock)
	fc.WaitForWatcherAndIncrement(10 * time.Second)
	time.Sleep(time.Second)

	if !conf.StandbyProcess.(*mockMonitorable).isActivated() {
		t.Fatal("process should be activated")
	}
}

func TestFailoverManagerFromChanSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	conf, failoverManager := failoverManager(ctx, t)

	// Set a lease on the key before we use it so that it's
	// inaccessible.
	response, err := nodeosmonitor.GetEtcdClient(t).Lease.Grant(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conf.KVClient.Put(ctx, conf.EtcdKey, "woo",
		clientv3.WithLease(response.ID)); err != nil {
		t.Fatal(err)
	}

	failoverManager.Start(ctx)
	time.Sleep(time.Second)

	if _, err := conf.KVClient.Delete(ctx, conf.EtcdKey); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)

	if !conf.ActiveProcess.(*mockMonitorable).isActivated() {
		t.Fatal("process should be activated")
	}
}
