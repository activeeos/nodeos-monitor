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

func failoverManager(t *testing.T) (*nodeosmonitor.FailoverConfig,
	*nodeosmonitor.FailoverManager) {
	clock := fakeclock.NewFakeClock(time.Now())
	client := nodeosmonitor.GetEtcdClient(t)
	id := uuid.New().String()
	key := uuid.New().String()
	activeProcess := &mockMonitorable{}
	standbyProcess := &mockMonitorable{}

	conf := &nodeosmonitor.FailoverConfig{
		ID:             id,
		EtcdKey:        key,
		Clock:          clock,
		LeaseClient:    client.Lease,
		WatcherClient:  client.Watcher,
		KVClient:       client.KV,
		ActiveProcess:  activeProcess,
		StandbyProcess: standbyProcess,
	}

	return conf, nodeosmonitor.NewFailoverManager(conf)
}

func TestFailoverManagerActivateImmediatelySuccess(t *testing.T) {
	conf, failoverManager := failoverManager(t)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	failoverManager.Start(ctx)
	if err := failoverManager.TryActivate(ctx); err != nil {
		t.Fatal(err)
	}

	if !conf.ActiveProcess.(*mockMonitorable).isActivated() {
		t.Fatal("process should be activated")
	}
}

func TestFailoverManagerActivateImmediatelyFailed(t *testing.T) {
	conf, failoverManager := failoverManager(t)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// Set a lease on the key before we use it so that it's
	// inaccessible.
	response, err := conf.LeaseClient.Grant(ctx, 100)
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
	conf, failoverManager := failoverManager(t)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

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
	conf, failoverManager := failoverManager(t)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// Set a lease on the key before we use it so that it's
	// inaccessible.
	response, err := conf.LeaseClient.Grant(ctx, 100)
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
	conf, failoverManager := failoverManager(t)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// Set a lease on the key before we use it so that it's
	// inaccessible.
	response, err := conf.LeaseClient.Grant(ctx, 100)
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
