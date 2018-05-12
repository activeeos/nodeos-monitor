package nodeosmonitor

import (
	"context"
	"testing"

	"time"

	"code.cloudfoundry.org/clock"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"github.com/sirupsen/logrus"
)

func GetEtcdClient(t *testing.T) *clientv3.Client {
	client, err := clientv3.NewFromURL("localhost:22379")
	if err != nil {
		t.Fatal(err)
	}

	return client
}

func TestEtcdLeaseManager(t *testing.T) {
	manager := NewEtcdLeaseManager(
		clock.NewClock(), 10, GetEtcdClient(t).Lease, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	go manager.Start(ctx)

	for i := 0; i < 10; i++ {
		leaseID, _ := manager.LeaseID()
		if leaseID != 0 {
			return
		}
		if leaseID == 0 {
			time.Sleep(1 * time.Second)
			continue
		}
	}

	t.Fatal("lease not attained")
}

func TestKeyChangeNotifier(t *testing.T) {
	key := "testkey!"
	client := GetEtcdClient(t)
	change := make(chan *mvccpb.KeyValue)
	notifier := NewKeyChangeNotifier(key, client.Watcher, change)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	go notifier.Start(ctx)

	client.KV.Put(ctx, key, "1")
	<-change

	client.KV.Put(ctx, key, "2")
	<-change

	client.KV.Put(ctx, key, "3")
	<-change
}

func TestEtcdMutex(t *testing.T) {
	client := GetEtcdClient(t)
	key := "testlock"

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	manager1NewLease := make(chan struct{})
	manager1 := NewEtcdLeaseManager(
		clock.NewClock(), 10, GetEtcdClient(t).Lease, manager1NewLease)
	go manager1.Start(ctx)

	manager2NewLease := make(chan struct{})
	manager2 := NewEtcdLeaseManager(
		clock.NewClock(), 10, GetEtcdClient(t).Lease, manager2NewLease)
	go manager2.Start(ctx)

	mutex1 := NewEtcdMutex("1", key, client.KV, manager1)
	mutex2 := NewEtcdMutex("2", key, client.KV, manager2)

	<-manager1NewLease
	<-manager2NewLease

	t.Run("first lock", func(t *testing.T) {
		locked, err := mutex1.Lock(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !locked {
			t.Fatal("should be locked")
		}
	})
	t.Run("second lock should succeed", func(t *testing.T) {
		locked, err := mutex1.Lock(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !locked {
			t.Fatal("should be locked")
		}
	})
	t.Run("different lock should fail", func(t *testing.T) {
		locked, err := mutex2.Lock(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if locked {
			t.Fatal("should not lock")
		}
	})
}

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}
