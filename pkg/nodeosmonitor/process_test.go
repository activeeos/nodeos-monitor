package nodeosmonitor

import (
	"context"
	"testing"
)

func TestProcessMonitorActivateWithFailure(t *testing.T) {
	failure := make(chan struct{})
	pm := NewProcessMonitor("/bin/ls", []string{"/"})
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
	pm := NewProcessMonitor("/bin/sleep", []string{"100"})
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

func TestRegexps(t *testing.T) {
	match := producedBlockRegexp.Match([]byte(`2378000ms thread-0   producer_plugin.cpp:1073      produce_block        ] Produced block 00000002c44770e8... #2 @ 2018-06-02T21:39:38.000 signed by eosio [trxs: 0, lib: 0, confirmed: 0]`))
	if !match {
		t.Fail()
	}
	match = receivedBlockRegexp.Match([]byte(`2730606ms thread-0   producer_plugin.cpp:290       on_incoming_block    ] Received block 6408521f7667f193... #156759 @ 2018-06-02T21:45:30.500 signed by komododragon [trxs: 0, lib: 156434, conf: 0, latency: 106 ms]`))
	if !match {
		t.Fail()
	}
}
