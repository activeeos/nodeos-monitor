package nodeosmonitor

import (
	"context"
	"os/exec"
	"sync"

	"bufio"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	_ Monitorable = &ProcessMonitor{}
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
	HandleFailure(ctx context.Context)
}

// ProcessMonitor monitors a process informing a handler when the
// process fails.
type ProcessMonitor struct {
	sync.Mutex
	path     string
	args     []string
	cmd      *exec.Cmd
	shutdown bool
}

// NewProcessMonitor returns a new instance of ProcessMonitor.
func NewProcessMonitor(path string, args []string) *ProcessMonitor {
	return &ProcessMonitor{
		path: path,
		args: args,
	}
}

// Activate starts the underlying process.
func (p *ProcessMonitor) Activate(ctx context.Context,
	failureHandler ProcessFailureHandler) error {
	p.Lock()
	defer p.Unlock()

	if p.cmd != nil {
		return errors.New("error process is already active")
	}

	cmd := exec.CommandContext(ctx, p.path, p.args...)

	// Print all stdin/stderr output to the logrus logger.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return errors.Wrapf(err, "error opening stdout pipe")
	}
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			logrus.Infof("wrapped process stdout: %s", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			logrus.WithError(err).Errorf("error scanning stdout")
		}
	}()

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return errors.Wrapf(err, "error opening stdout pipe")
	}
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			logrus.Infof("wrapped process stderr: %s", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			logrus.WithError(err).Errorf("error scanning stderr")
		}
	}()

	// Start the command
	if err := cmd.Start(); err != nil {
		return errors.Wrapf(err, "error starting command")
	}
	p.cmd = cmd
	p.shutdown = false

	// Create exited channel so that we can wait for this in next
	// goroutine.
	exitedChan := make(chan struct{})
	go func() {
		if err := cmd.Wait(); err != nil {
			logrus.WithError(err).Errorf("error waiting for command to finish execution")
		}
		close(exitedChan)
	}()

	// This goroutine waits until the process fails, notifying the
	// failure handler.
	go func() {
		select {
		case <-exitedChan:
		case <-ctx.Done():
			return
		}

		// Check that this is a random failure, not a shutdown.
		p.Lock()
		if p.shutdown {
			return
		}
		p.Unlock()

		failureHandler.HandleFailure(ctx)
	}()

	return nil
}

// Shutdown shuts the current process down.
func (p *ProcessMonitor) Shutdown(ctx context.Context) error {
	p.Lock()
	p.Unlock()

	// This is to let goroutines know that this isn't a random
	// failure.
	p.shutdown = true

	if p.cmd == nil {
		return errors.New("process monitor doesn't have an underlying process")
	}
	if !p.cmd.ProcessState.Exited() {
		if err := p.cmd.Process.Kill(); err != nil {
			return errors.Wrapf(err, "error killing the process")
		}
	}

	p.shutdown = false
	p.cmd = nil

	return nil
}
