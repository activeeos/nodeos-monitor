package nodeosmonitor

import (
	"context"
	"time"

	"sync/atomic"

	"code.cloudfoundry.org/clock"
	"github.com/coreos/etcd/clientv3"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	// ErrNoLease is returned when a lease isn't active for a
	// LeaseManager.
	ErrNoLease = errors.New("error: lease not active")
)

// LeaseManager always maintains an Etcd lease, calling callbacks when
// the current lease is lost.
type LeaseManager struct {
	config             *Config
	clock              clock.Clock
	leaseClient        clientv3.Lease
	id                 clientv3.LeaseID
	lostLeaseCallbacks []func()
}

// AddLostLeaseCallbacks adds a function to the list of functions
// called when a lease is called.
func (l *LeaseManager) AddLostLeaseCallbacks(f func()) {
	l.lostLeaseCallbacks = append(l.lostLeaseCallbacks, f)
}

// Start begins the process of attaining and renewing leases.
func (l *LeaseManager) Start(ctx context.Context) {
	for {
		if err := l.attainLease(ctx); err != nil {
			logrus.WithError(err).Errorf("error attaining lease")
			l.clock.Sleep(time.Second)
			continue
		}
		if err := l.manageKeepAlive(ctx); err != nil {
			logrus.WithError(err).Errorf("error managing lease keep-alive")
			l.clock.Sleep(time.Second)
			continue
		}
	}
}

// LeaseID returns the current Etcd lease ID.
func (l *LeaseManager) LeaseID() (clientv3.LeaseID, error) {
	if l.id == 0 {
		return 0, ErrNoLease
	}

	return l.id, nil
}

func (l *LeaseManager) attainLease(ctx context.Context) error {
	response, err := l.leaseClient.Grant(ctx, l.config.LockTTLSeconds)
	if err != nil {
		return errors.Wrapf(err, "error granting lease")
	}

	atomic.StoreInt64((*int64)(&l.id), int64(response.ID))

	return nil
}

func (l *LeaseManager) manageKeepAlive(ctx context.Context) error {
	keepAlive, err := l.leaseClient.KeepAlive(ctx, l.id)
	if err != nil {
		return errors.Wrapf(err, "error starting lease keep alive goroutine")
	}

	for {
		_, ok := <-keepAlive
		if !ok {
			break
		}
	}

	atomic.StoreInt64((*int64)(&l.id), 0)

	for _, callback := range l.lostLeaseCallbacks {
		callback()
	}

	return nil
}

// Architecture
// Maintain lease
// When the lease on the key is removed, try to claim the key

// func (f *FailoverManager)
