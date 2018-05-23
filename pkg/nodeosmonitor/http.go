package nodeosmonitor

import (
	"net/http"

	"github.com/sirupsen/logrus"
)

// NewActiveCheckServer creates a new ActiveCheckServer.
func NewActiveCheckServer(monitor *ProcessMonitor) *ActiveCheckServer {
	return &ActiveCheckServer{
		monitor: monitor,
	}
}

// ActiveCheckServer is an HTTP server that's used to implement a
// health check around a ProcessMonitor. If the process is active, an
// HTTP 200 is returned.
type ActiveCheckServer struct {
	monitor *ProcessMonitor
}

// ServeHTTP returns an HTTP 200 if the underlying ProcessMonitor is
// active.
func (p *ActiveCheckServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logrus.Debugf("received HTTP health check for process monitor")

	active := p.monitor.IsActive()

	if active {
		logrus.Debugf("health check response: %d", http.StatusOK)
		return
	}

	logrus.Debugf("health check response: %d", http.StatusExpectationFailed)
	w.WriteHeader(http.StatusExpectationFailed)
}
