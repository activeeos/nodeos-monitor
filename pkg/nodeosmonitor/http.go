package nodeosmonitor

import "net/http"

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
	active := p.monitor.IsActive()

	if active {
		return
	}

	w.WriteHeader(http.StatusExpectationFailed)
}
