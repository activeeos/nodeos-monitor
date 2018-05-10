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

	restartDelay = 10 * time.Second
)

// Process is a type that can be activated and shutdown. In this
// codebase, a Process is usually an actual OS process.
type Process interface {
	Activate(ProcessFailureHandler)
	Shutdown()
}

// ProcessFailureHandler is something that's called when a process
// fails.
type ProcessFailureHandler interface {
	HandleFailure(ctx context.Context)
}

// FailoverManager manages a failover system for a process using
// Etcd. The active version of a Process is activated when a lock is
// attained on an Etcd key. The standby version of a process is
// activated when the lock is lost or if it isn't
// attainable. Additionally, FailoverManager supports being notified
// of a downstream failure, which causes it to lose its Etcd lease and
// restart.
type FailoverManager struct {
	sync.Mutex
	ctx            context.Context
	id             string
	key            string
	clock          clock.Clock
	leaseClient    clientv3.Lease
	watcherClient  clientv3.Watcher
	kvClient       clientv3.KV
	activeProcess  Process
	standbyProcess Process

	// Dynamic fields
	currentState int
	cancel       func()
	leaseManager *EtcdLeaseManager
	mutex        *EtcdMutex
	notifier     *KeyChangeNotifier
}

// Start begins the failover process.
func (f *FailoverManager) Start(ctx context.Context) {
	f.ctx = ctx
	f.init()
}

func (f *FailoverManager) init() {
	f.Lock()
	defer f.Unlock()

	ctx, cancel := context.WithCancel(f.ctx)
	f.cancel = cancel

	lostLeaseChan := make(chan struct{})
	f.leaseManager = NewEtcdLeaseManager(f.clock,
		defaultLeaseTTLSeconds, f.leaseClient, lostLeaseChan)
	go f.leaseManager.Start(ctx)
	go f.handleLostLease(ctx, lostLeaseChan)

	notificationChan := make(chan *mvccpb.KeyValue)
	f.notifier = NewKeyChangeNotifier(f.key, f.watcherClient, notificationChan)
	go f.notifier.Start(ctx)
	go f.tryActiveFromChan(ctx, notificationChan)

	f.mutex = NewEtcdMutex(f.id, f.key, f.kvClient, f.leaseManager)

	if err := f.tryActivate(ctx); err != nil {
		logrus.WithError(err).Errorf("error trying to activate on init")
	}
}

func (f *FailoverManager) tryActiveFromChan(ctx context.Context, c chan *mvccpb.KeyValue) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c:
		}

		if err := f.tryActivate(ctx); err != nil {
			logrus.WithError(err).Errorf("error trying to activate from failover")
		}
	}
}

func (f *FailoverManager) tryActivate(ctx context.Context) error {
	locked, err := f.mutex.Lock(ctx)
	if err != nil {
		return errors.Wrapf(err, "error trying to lock Etcd mutex")
	}

	if locked {
		f.handleActive(ctx)
	} else {
		f.handleStandby(ctx)
	}

	return nil
}

func (f *FailoverManager) handleLostLease(ctx context.Context, c chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c:
		}

		f.cancel()
		f.init()
	}
}

func (f *FailoverManager) shutdownProcesses() {
	if f.currentState == failoverStateActive {
		f.activeProcess.Shutdown()
	}
	if f.currentState == failoverStateStandby {
		f.standbyProcess.Shutdown()
	}

	f.currentState = 0
}

// HandleFailure is called by an external process in order to restart
// the FailoverManager, losing any currently active leases.
func (f *FailoverManager) HandleFailure(ctx context.Context) {
	f.cancel()
	f.shutdownProcesses()

	f.clock.Sleep(restartDelay)

	f.init()
}

func (f *FailoverManager) handleActive(ctx context.Context) {
	f.Lock()
	defer f.Unlock()

	if f.currentState == failoverStateActive {
		return
	}

	f.shutdownProcesses()

	f.currentState = failoverStateActive
	f.activeProcess.Activate(f)
}

func (f *FailoverManager) handleStandby(ctx context.Context) {
	f.Lock()
	defer f.Unlock()

	if f.currentState == failoverStateStandby {
		return
	}

	f.shutdownProcesses()

	f.currentState = failoverStateStandby
	f.standbyProcess.Activate(f)
}
