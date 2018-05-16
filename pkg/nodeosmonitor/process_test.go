package nodeosmonitor_test

import (
	"context"
	"testing"

	"github.com/activeeos/nodeos-monitor/pkg/nodeosmonitor"
)

func TestProcessMonitorActivateWithFailure(t *testing.T) {
	failure := make(chan struct{})
	pm := nodeosmonitor.NewProcessMonitor("/bin/ls", []string{"/"})
	failureHandler := &mockFailureHandler{failure}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	if err := pm.Activate(ctx, failureHandler); err != nil {
		t.Fatal(err)
	}

	<-failure
}

func TestProcessMonitorActivateThenShutdown(t *testing.T) {
	failure := make(chan struct{})
	pm := nodeosmonitor.NewProcessMonitor("/bin/sleep", []string{"100"})
	failureHandler := &mockFailureHandler{failure}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	if err := pm.Activate(ctx, failureHandler); err != nil {
		t.Fatal(err)
	}
	if err := pm.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}

	select {
	case <-failure:
		t.Fatal("failure handler should not have been called")
	default:
	}

	// Activate and shutdown again to make sure that we can do this
	// multiple times.
	if err := pm.Activate(ctx, failureHandler); err != nil {
		t.Fatal(err)
	}
	if err := pm.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}

	select {
	case <-failure:
		t.Fatal("failure handler should not have been called")
	default:
	}
}
