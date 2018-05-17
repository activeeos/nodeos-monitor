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
	failoverStateActive  = 1
	failoverStateStandby = 2

	periodicActivationInterval = 5 * time.Second
)

// FailoverManager manages a failover system for a process using
// Etcd. The active version of a Monitorable is activated when a lock
// is attained on an Etcd key. The standby version of a process is
// activated when the lock is lost or if it isn't
// attainable. Additionally, FailoverManager supports being notified
// of a downstream failure, which causes it to lose its Etcd lease and
// restart.
type FailoverManager struct {
	sync.Mutex
	id             string
	key            string
	clock          clock.Clock
	leaseClient    clientv3.Lease
	watcherClient  clientv3.Watcher
	kvClient       clientv3.KV
	activeProcess  Monitorable
	standbyProcess Monitorable

	// Internal fields
	currentState  int
	leaseManager  *EtcdLeaseManager
	mutex         *EtcdMutex
	notifier      *KeyChangeNotifier
	leaseAttained chan struct{}
}

// FailoverConfig contains the parameters for a FailoverManager.
type FailoverConfig struct {
	ID             string
	EtcdKey        string
	Clock          clock.Clock
	LeaseClient    clientv3.Lease
	WatcherClient  clientv3.Watcher
	KVClient       clientv3.KV
	ActiveProcess  Monitorable
	StandbyProcess Monitorable
}

// NewFailoverManager instantiates a new FailoverManager.
func NewFailoverManager(config *FailoverConfig) *FailoverManager {
	return &FailoverManager{
		id:             config.ID,
		key:            config.EtcdKey,
		clock:          config.Clock,
		leaseClient:    config.LeaseClient,
		watcherClient:  config.WatcherClient,
		kvClient:       config.KVClient,
		activeProcess:  config.ActiveProcess,
		standbyProcess: config.StandbyProcess,
		leaseAttained:  make(chan struct{}, 1),
	}
}

// Start begins the FailoverManager process. The process runs
// asynchronously until the context is canceled.
func (f *FailoverManager) Start(ctx context.Context) {
	logrus.Infof("starting failover manager")

	f.leaseManager = NewEtcdLeaseManager(f.clock,
		defaultLeaseTTLSeconds, f.leaseClient, f.leaseAttained)
	go f.leaseManager.Start(ctx)

	f.mutex = NewEtcdMutex(f.id, f.key, f.kvClient, f.leaseManager)

	notificationChan := make(chan *mvccpb.KeyValue)
	f.notifier = NewKeyChangeNotifier(f.key, f.watcherClient, notificationChan)
	go f.notifier.Start(ctx)
	go f.tryActivateFromChan(ctx, notificationChan)
	go f.tryActivatePeriodically(ctx)

	logrus.Debugf("started failover manager")
}

func (f *FailoverManager) tryActivatePeriodically(ctx context.Context) {
	ticker := f.clock.NewTicker(periodicActivationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
		}

		logrus.Debugf("trying to activate process periodically")

		if err := f.tryActivate(ctx); err != nil {
			logrus.WithError(err).Errorf("error trying to activate process periodically")
		}
	}
}

func (f *FailoverManager) tryActivateFromChan(ctx context.Context, c chan *mvccpb.KeyValue) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c:
		}

		logrus.Debugf("trying to activate process from channel notification")

		if err := f.tryActivate(ctx); err != nil {
			logrus.WithError(err).Errorf("error trying to activate process from chan")
		}
	}
}

// TryActivate forces an activation, waiting for a least to be
// attained.
func (f *FailoverManager) TryActivate(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	case <-f.leaseAttained:
	}

	if err := f.tryActivate(ctx); err != nil {
		return errors.Wrapf(err, "error attempting to activate process")
	}

	return nil
}

func (f *FailoverManager) tryActivate(ctx context.Context) error {
	f.Lock()
	defer f.Unlock()

	locked, err := f.mutex.Lock(ctx)
	if err != nil {
		return errors.Wrapf(err, "error trying to lock Etcd mutex")
	}

	if locked {
		if f.currentState == failoverStateActive {
			return nil
		}
		if err := f.handleActive(ctx); err != nil {
			return errors.Wrapf(err, "error activating active process")
		}
	} else {
		if f.currentState == failoverStateStandby {
			return nil
		}
		if err := f.handleStandby(ctx); err != nil {
			return errors.Wrapf(err, "error activating standby process")
		}
	}

	return nil
}

func (f *FailoverManager) shutdownProcesses(ctx context.Context) error {
	logrus.Debugf("shutting down existing processes")

	var currentProcess Monitorable
	if f.currentState == failoverStateActive {
		currentProcess = f.activeProcess
	}
	if f.currentState == failoverStateStandby {
		currentProcess = f.standbyProcess
	}
	if currentProcess == nil {
		return nil
	}

	if err := currentProcess.Shutdown(ctx); err != nil {
		return errors.Wrapf(err, "error shutting down underlying process")
	}

	f.currentState = 0

	logrus.Debugf("shut down existing processes")

	return nil
}

func (f *FailoverManager) currentProcess() Monitorable {
	switch f.currentState {
	case failoverStateActive:
		return f.activeProcess
	case failoverStateStandby:
		return f.standbyProcess
	}

	return nil
}

// HandleFailure is called by an external process in order to restart
// the FailoverManager, losing any currently active leases.
func (f *FailoverManager) HandleFailure(ctx context.Context, m Monitorable) {
	f.Lock()
	defer f.Unlock()

	if m != f.currentProcess() {
		return
	}

	logrus.Infof("downstream process failed, revoking lease")

	if err := f.shutdownProcesses(ctx); err != nil {
		logrus.WithError(err).Errorf("error shutting down processes while handling failure")
	}

	if err := f.leaseManager.RevokeCurrentLease(ctx); err != nil {
		logrus.WithError(err).Errorf("error revoking lease while handling process failure")
	}

	logrus.Infof("revoked lease")
}

func (f *FailoverManager) handleActive(ctx context.Context) error {
	logrus.Debugf("activating active process")

	if f.currentState == failoverStateActive {
		return errors.New("error: process already active")
	}

	if err := f.shutdownProcesses(ctx); err != nil {
		return errors.Wrapf(err,
			"error shutting down processes before activating active process")
	}

	f.currentState = failoverStateActive
	if err := f.activeProcess.Activate(ctx, f); err != nil {
		return errors.Wrapf(err, "error activating active process")
	}

	logrus.Infof("activated active process")

	return nil
}

func (f *FailoverManager) handleStandby(ctx context.Context) error {
	logrus.Debugf("activating standby process")

	if f.currentState == failoverStateStandby {
		return errors.New("error: standby process already exists")
	}

	if err := f.shutdownProcesses(ctx); err != nil {
		return errors.Wrapf(err,
			"error shutting down processes before activating standby process")
	}

	f.currentState = failoverStateStandby
	if err := f.standbyProcess.Activate(ctx, f); err != nil {
		return errors.Wrapf(err, "error activating standyby process")
	}

	logrus.Infof("activated standby process")

	return nil
}
