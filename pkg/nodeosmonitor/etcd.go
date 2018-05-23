package nodeosmonitor

import (
	"context"
	"sync"
	"time"

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
	mutex       *sync.Mutex
	clock       clock.Clock
	leaseClient clientv3.Lease
	id          clientv3.LeaseID
	wg          *sync.WaitGroup
	gotLease    chan struct{}
	shutdown    chan struct{}
}

// NewEtcdLeaseManager instantiates a new EtcdLeaseManager.
func NewEtcdLeaseManager(clock clock.Clock, leaseClient clientv3.Lease) *EtcdLeaseManager {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	return &EtcdLeaseManager{
		mutex:       &sync.Mutex{},
		clock:       clock,
		leaseClient: leaseClient,
		wg:          wg,
		gotLease:    make(chan struct{}),
		shutdown:    make(chan struct{}),
	}
}

// AfterLease returns a channel that's closed when a lease has been
// attained.
func (l *EtcdLeaseManager) AfterLease(ctx context.Context) chan struct{} {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.gotLease
}

// RevokeCurrentLease revokes the current lease. This makes it so that
// the LeaseManager requests a new lease.
func (l *EtcdLeaseManager) RevokeCurrentLease(ctx context.Context) error {
	logrus.Infof("revoking Etcd lease")

	if _, err := l.leaseClient.Revoke(ctx, l.id); err != nil {
		return errors.Wrapf(err, "error revoking current lease")
	}

	l.id = 0

	return nil
}

// Shutdown shuts down the lease manager, revoking all leases.
func (l *EtcdLeaseManager) Shutdown(ctx context.Context) {
	logrus.Infof("shutting down Etcd lease manager")

	close(l.shutdown)

	if l.id != 0 {
		if err := l.RevokeCurrentLease(ctx); err != nil {
			logrus.WithError(err).Errorf("error revoking lease while shutting down")
		}
	}

	l.wg.Wait()

	logrus.Debug("finished shutting down Etcd lease manager")
}

// Start begins the process of attaining and renewing leases.
func (l *EtcdLeaseManager) Start(ctx context.Context) {
	defer l.wg.Done()
	logrus.Info("starting lease manager")

	for {
		logrus.Debugf("trying to attain Etcd lease")

		select {
		case <-ctx.Done():
			return
		case <-l.shutdown:
			return
		default:
		}

		logrus.Info("attaining Etcd lease")

		if err := l.attainLease(ctx); err != nil {
			logrus.WithError(err).Errorf("error attaining lease")
			select {
			case <-ctx.Done():
				return
			case <-l.shutdown:
				return
			case <-time.After(leaseRecoveryDelay):
			}
			continue
		}

		logrus.Infof("attained Etcd lease: %d", l.id)
		close(l.gotLease)

		if err := l.manageKeepAlive(ctx); err != nil {
			logrus.WithError(err).Errorf("error managing lease keep-alive")
		}

		logrus.Warn("lost Etcd lease")

		l.mutex.Lock()
		l.gotLease = make(chan struct{})
		l.mutex.Unlock()

		select {
		case <-ctx.Done():
			return
		case <-l.shutdown:
			return
		case <-time.After(leaseRecoveryDelay):
		}
	}
}

// LeaseID returns the current Etcd lease ID.
func (l *EtcdLeaseManager) LeaseID() (clientv3.LeaseID, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.id == 0 {
		return 0, ErrNoLease
	}

	return l.id, nil
}

func (l *EtcdLeaseManager) attainLease(ctx context.Context) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	response, err := l.leaseClient.Grant(ctx, defaultLeaseTTLSeconds)
	if err != nil {
		return errors.Wrapf(err, "error granting lease")
	}

	l.id = response.ID

	return nil
}

func (l *EtcdLeaseManager) manageKeepAlive(ctx context.Context) error {
	keepAlive, err := l.leaseClient.KeepAlive(ctx, l.id)
	if err != nil {
		return errors.Wrapf(err, "error starting lease keep alive goroutine")
	}

Loop:
	for {
		select {
		case _, ok := <-keepAlive:
			if !ok {
				break Loop
			}
		case <-ctx.Done():
			return nil
		case <-l.shutdown:
			return nil
		}

		logrus.Debug("received Etcd lease keep-alive message")
	}

	logrus.Debug("Etcd keep-alive ended")

	l.mutex.Lock()
	l.id = 0
	l.mutex.Unlock()

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
	logrus.Info("starting Etcd key change notifier")

	watchChan := k.watcherClient.Watch(ctx, k.key)

Loop:
	for {
		var response clientv3.WatchResponse

		select {
		case r, ok := <-watchChan:
			if !ok {
				break Loop
			}
			response = r
		case <-ctx.Done():
			logrus.Info("stopped Etcd key change notifier")
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
