package nodeosmonitor_test

import (
	"context"
	"sync"

	"github.com/activeeos/nodeos-monitor/pkg/nodeosmonitor"
)

// mockMonitorable is a mock for Monitorable. We need this because the
// Testify mock is racy.
type mockMonitorable struct {
	sync.Mutex
	activated bool
	shutdown  bool
}

func (m *mockMonitorable) Activate(_ context.Context, _ nodeosmonitor.ProcessFailureHandler) error {
	m.Lock()
	defer m.Unlock()
	m.activated = true
	return nil
}

func (m *mockMonitorable) Shutdown(_ context.Context) error {
	m.Lock()
	defer m.Unlock()
	m.shutdown = true
	return nil
}

func (m *mockMonitorable) isActivated() bool {
	m.Lock()
	defer m.Unlock()
	return m.activated
}

func (m *mockMonitorable) isShutdown() bool {
	m.Lock()
	defer m.Unlock()
	return m.shutdown
}
