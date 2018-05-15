package nodeosmonitor_test

import (
	"context"
	"testing"
	"time"

	"code.cloudfoundry.org/clock/fakeclock"
	"github.com/activeeos/nodeos-monitor/pkg/nodeosmonitor"
	"github.com/activeeos/nodeos-monitor/pkg/nodeosmonitor/mocks"
	"github.com/coreos/etcd/clientv3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

func failoverManager(t *testing.T) (*nodeosmonitor.FailoverConfig,
	*nodeosmonitor.FailoverManager) {
	clock := fakeclock.NewFakeClock(time.Now())
	client := nodeosmonitor.GetEtcdClient(t)
	id := uuid.New().String()
	key := uuid.New().String()
	activeProcess := &mocks.Process{}
	standbyProcess := &mocks.Process{}

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

	conf.ActiveProcess.(*mocks.Process).On("Activate",
		mock.Anything, failoverManager).Return()
	defer conf.ActiveProcess.(*mocks.Process).AssertExpectations(t)

	failoverManager.Start(ctx)
	if err := failoverManager.TryActivate(ctx); err != nil {
		t.Fatal(err)
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

	conf.StandbyProcess.(*mocks.Process).On("Activate",
		mock.Anything, failoverManager).Return()
	defer conf.ActiveProcess.(*mocks.Process).AssertExpectations(t)

	failoverManager.Start(ctx)
	if err := failoverManager.TryActivate(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestFailoverManagerActivatePeriodicallySuccess(t *testing.T) {
	conf, failoverManager := failoverManager(t)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	conf.ActiveProcess.(*mocks.Process).On("Activate",
		mock.Anything, failoverManager).Return()
	defer conf.ActiveProcess.(*mocks.Process).AssertExpectations(t)

	// TODO: figure out how to remove time dependency from here, maybe
	// by watching the Etcd key.

	failoverManager.Start(ctx)
	time.Sleep(time.Second)

	fc := conf.Clock.(*fakeclock.FakeClock)
	fc.WaitForWatcherAndIncrement(10 * time.Second)
	time.Sleep(time.Second)
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

	conf.StandbyProcess.(*mocks.Process).On("Activate",
		mock.Anything, failoverManager).Return()
	defer conf.ActiveProcess.(*mocks.Process).AssertExpectations(t)

	// TODO: figure out how to remove time dependency from here, maybe
	// by watching the Etcd key.

	failoverManager.Start(ctx)
	time.Sleep(time.Second)

	fc := conf.Clock.(*fakeclock.FakeClock)
	fc.WaitForWatcherAndIncrement(10 * time.Second)
	time.Sleep(time.Second)
}

func TestFailoverManagerFromChanImmediatelySuccess(t *testing.T) {
	conf, failoverManager := failoverManager(t)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	conf.ActiveProcess.(*mocks.Process).On("Activate",
		mock.Anything, failoverManager).Return()
	defer conf.ActiveProcess.(*mocks.Process).AssertExpectations(t)

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
}
