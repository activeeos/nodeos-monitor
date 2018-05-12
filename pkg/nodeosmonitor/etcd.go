package nodeosmonitor

import (
	"context"
	"time"

	"sync/atomic"

	"code.cloudfoundry.org/clock"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	leaseRecoveryDelay     = 10 * time.Second
	defaultLeaseTTLSeconds = 10
)

var (
	// ErrNoLease is returned when a lease isn't active for a
	// EtcdLeaseManager.
	ErrNoLease = errors.New("error: lease not active")
)

// EtcdLeaseManager always maintains an Etcd lease, notifying through a
// channel when the current lease is lost.
type EtcdLeaseManager struct {
	lockTTLSeconds     int64
	clock              clock.Clock
	leaseClient        clientv3.Lease
	id                 clientv3.LeaseID
	firstLeaseChan     chan struct{}
	firstLeaseAttained bool
}

// NewEtcdLeaseManager instantiates a new EtcdLeaseManager.
func NewEtcdLeaseManager(clock clock.Clock, lockTTLSeconds int64,
	leaseClient clientv3.Lease, firstLeaseChan chan struct{}) *EtcdLeaseManager {
	return &EtcdLeaseManager{
		clock:          clock,
		leaseClient:    leaseClient,
		lockTTLSeconds: lockTTLSeconds,
		firstLeaseChan: firstLeaseChan,
	}
}

// RevokeCurrentLease revokes the current lease. This makes it so that
// the LeaseManager requests a new lease.
func (l *EtcdLeaseManager) RevokeCurrentLease(ctx context.Context) error {
	if _, err := l.leaseClient.Revoke(ctx, l.id); err != nil {
		return errors.Wrapf(err, "error revoking current lease")
	}

	return nil
}

// Start begins the process of attaining and renewing leases.
func (l *EtcdLeaseManager) Start(ctx context.Context) {
	logrus.Info("starting lease manager")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		logrus.Info("attaining Etcd lease")

		if err := l.attainLease(ctx); err != nil {
			logrus.WithError(err).Errorf("error attaining lease")
			l.clock.Sleep(leaseRecoveryDelay)
			continue
		}

		logrus.Infof("attained Etcd lease: %d", l.id)

		if err := l.manageKeepAlive(ctx); err != nil {
			logrus.WithError(err).Errorf("error managing lease keep-alive")
		}

		logrus.Warn("lost Etcd lease")
		l.clock.Sleep(leaseRecoveryDelay)
	}
}

// LeaseID returns the current Etcd lease ID.
func (l *EtcdLeaseManager) LeaseID() (clientv3.LeaseID, error) {
	if l.id == 0 {
		return 0, ErrNoLease
	}

	return l.id, nil
}

func (l *EtcdLeaseManager) attainLease(ctx context.Context) error {
	response, err := l.leaseClient.Grant(ctx, l.lockTTLSeconds)
	if err != nil {
		return errors.Wrapf(err, "error granting lease")
	}

	atomic.StoreInt64((*int64)(&l.id), int64(response.ID))

	if !l.firstLeaseAttained && l.firstLeaseChan != nil {
		l.firstLeaseChan <- struct{}{}
		l.firstLeaseAttained = true
	}

	return nil
}

func (l *EtcdLeaseManager) manageKeepAlive(ctx context.Context) error {
	keepAlive, err := l.leaseClient.KeepAlive(ctx, l.id)
	if err != nil {
		return errors.Wrapf(err, "error starting lease keep alive goroutine")
	}

	for {
		_, ok := <-keepAlive
		if !ok {
			break
		}

		logrus.Debug("received Etcd lease keep-alive message")
	}

	logrus.Debug("Etcd keep-alive ended")

	atomic.StoreInt64((*int64)(&l.id), 0)

	return nil
}

// KeyChangeNotifier notifies a channel when an Etcd key is modified.
type KeyChangeNotifier struct {
	key           string
	watcherClient clientv3.Watcher
	changeChan    chan *mvccpb.KeyValue
}

// NewKeyChangeNotifier instantiates a new KeyChangeNotifier.
func NewKeyChangeNotifier(key string, watcherClient clientv3.Watcher,
	changeChan chan *mvccpb.KeyValue) *KeyChangeNotifier {
	return &KeyChangeNotifier{
		key:           key,
		watcherClient: watcherClient,
		changeChan:    changeChan,
	}
}

// Start begins the key change notification process.
func (k *KeyChangeNotifier) Start(ctx context.Context) {
	watchChan := k.watcherClient.Watch(ctx, k.key)
	for {
		response, ok := <-watchChan
		if !ok {
			break
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		for _, event := range response.Events {
			logrus.Infof("detected Etcd key change for %s", k.key)
			k.changeChan <- event.Kv
		}
	}
}

// EtcdMutex provides a mutex on Etcd that locks based on a key.
type EtcdMutex struct {
	id           string
	key          string
	kvClient     clientv3.KV
	leaseManager *EtcdLeaseManager
}

// NewEtcdMutex creates a new EtcdMutex.
func NewEtcdMutex(id, key string, kvClient clientv3.KV,
	leaseManager *EtcdLeaseManager) *EtcdMutex {
	return &EtcdMutex{
		id:           id,
		key:          key,
		kvClient:     kvClient,
		leaseManager: leaseManager,
	}
}

// Lock attempts to lock around a key on Etcd, return a boolean
// representing whether the lock was attained.
func (e *EtcdMutex) Lock(ctx context.Context) (bool, error) {
	leaseID, err := e.leaseManager.LeaseID()
	if err != nil {
		return false, errors.Wrapf(err, "error getting lease ID")
	}

	_, err = e.kvClient.Txn(ctx).If(
		clientv3.Compare(clientv3.LeaseValue(e.key), "=", 0),
	).Then(
		clientv3.OpPut(e.key, e.id, clientv3.WithLease(leaseID)),
	).Commit()
	if err != nil {
		return false, errors.Wrapf(err, "error trying to attain lock")
	}

	getResponse, err := e.kvClient.Get(ctx, e.key)
	if err != nil {
		return false, errors.Wrapf(err, "error getting lock key")
	}
	if len(getResponse.Kvs) != 1 {
		return false, errors.Errorf("error: invalid number of KV pairs returned: %d",
			len(getResponse.Kvs))
	}

	// TODO: look into whether we can figure this out from the Txn
	// response.
	kv := getResponse.Kvs[0]
	return string(kv.Value) == e.id, nil
}
