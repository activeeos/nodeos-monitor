package nodeosmonitor

import (
	"context"
	"io"
	"os/exec"
	"regexp"
	"sync"
	"syscall"

	"bufio"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	_ Monitorable = &ProcessMonitor{}

	receivedBlockRegexp = regexp.MustCompile(`.*Received block.*signed by.*`)
	producedBlockRegexp = regexp.MustCompile(`.*Produced block.*signed by.*`)
)

// Monitorable is a type that can be activated and shutdown. In this
// codebase, a Monitorable is usually an actual OS process.
type Monitorable interface {
	Activate(context.Context, ProcessFailureHandler) error
	Shutdown(context.Context) error
}

// ProcessFailureHandler is something that's called when a process
// fails.
type ProcessFailureHandler interface {
	HandleFailure(ctx context.Context, m Monitorable)
}

// ProcessMonitor monitors a process informing a handler when the
// process fails.
type ProcessMonitor struct {
	mutex    *sync.Mutex
	path     string
	args     []string
	cmd      *exec.Cmd
	shutdown chan struct{}
	exited   chan struct{}
	active   bool
}

// NewProcessMonitor returns a new instance of ProcessMonitor.
func NewProcessMonitor(path string, args []string) *ProcessMonitor {
	return &ProcessMonitor{
		mutex: &sync.Mutex{},
		path:  path,
		args:  args,
	}
}

func manageWrappedProcessOut(cmd *exec.Cmd) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return errors.Wrapf(err, "error opening stdout pipe")
	}
	go func() {
		reader := bufio.NewReaderSize(stdout, 64*1024)
		for {
			line, _, err := reader.ReadLine()
			if err == io.EOF {
				return
			}
			if err != nil {
				logrus.WithError(err).Errorf("error reading stdout")
				return
			}
			extractMetricsFromLine(line)
			logrus.WithField("wrapped-process-stdout", string(line)).Info()
		}
	}()

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return errors.Wrapf(err, "error opening stdout pipe")
	}
	go func() {
		reader := bufio.NewReaderSize(stderr, 64*1024)
		for {
			line, _, err := reader.ReadLine()
			if err == io.EOF {
				return
			}
			if err != nil {
				logrus.WithError(err).Errorf("error reading stderr")
				return
			}
			extractMetricsFromLine(line)
			logrus.WithField("wrapped-process-stderr", string(line)).Info()
		}
	}()

	return nil
}

// IsActive returns true if the underlying process is active.
func (p *ProcessMonitor) IsActive() bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return p.active
}

func (p *ProcessMonitor) waitForProcessExit() {
	if err := p.cmd.Wait(); err != nil {
		logrus.WithError(err).Errorf("error waiting for command to finish executing")
		// TODO: do something if the cmd doesn't exit.
	}

	close(p.exited)
}

func (p *ProcessMonitor) handleProcessExit(ctx context.Context, failureHandler ProcessFailureHandler) {
	select {
	case <-p.shutdown:
		return
	case <-ctx.Done():
		return
	case <-p.exited:
	}

	logrus.Debugf("detected process failure %v", p.path)

	p.mutex.Lock()
	p.active = false
	p.mutex.Unlock()

	// Cleanup after the process if this is a random failure.
	if err := p.Shutdown(ctx); err != nil {
		logrus.WithError(err).Errorf("error shutting down process")
	}

	failureHandler.HandleFailure(ctx, p)
}

// Activate starts the underlying process.
func (p *ProcessMonitor) Activate(ctx context.Context,
	failureHandler ProcessFailureHandler) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	logrus.Debugf("activating process %v with args: %v", p.path, p.args)

	if p.cmd != nil {
		return errors.New("error: process is already active")
	}

	cmd := exec.Command(p.path, p.args...)

	if err := manageWrappedProcessOut(cmd); err != nil {
		return errors.Wrapf(err, "error managing wrapped process output")
	}

	if err := cmd.Start(); err != nil {
		return errors.Wrapf(err, "error starting command")
	}

	p.cmd = cmd
	p.shutdown = make(chan struct{})
	p.exited = make(chan struct{})
	p.active = true

	go p.waitForProcessExit()
	go p.handleProcessExit(ctx, failureHandler)

	return nil
}

// Shutdown shuts the current process down.
func (p *ProcessMonitor) Shutdown(ctx context.Context) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	logrus.Debugf("shutting down process %v", p.path)

	if p.cmd == nil {
		return nil
	}

	if p.cmd.ProcessState == nil {
		logrus.Debugf("sending SIGTERM to process %v", p.path)

		if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			// TODO: maybe do a kill -9 here if the process doesn't
			// shut down cleanly after a timeout?
			return errors.Wrapf(err, "error killing the process")
		}

		logrus.Debugf("waiting for process %v to exit", p.path)

		<-p.exited
		logrus.Debugf("killed process %v", p.path)
	}

	close(p.shutdown)
	p.active = false
	p.cmd = nil

	logrus.Debugf("shut down process %v", p.path)

	return nil
}

func extractMetricsFromLine(line []byte) {
	if receivedBlockRegexp.Match(line) {
		metrics.BlocksReceived.Inc()
	}
	if producedBlockRegexp.Match(line) {
		metrics.BlocksProduced.Inc()
	}
}
