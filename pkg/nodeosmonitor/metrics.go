package nodeosmonitor

import "github.com/prometheus/client_golang/prometheus"

// metrics is the globally accessible version of our
// MetricsAggregator.
var metrics = NewMetricsAggregator()

// MetricsAggregator contains the different Prometheus metrics that
// the application tracks.
type MetricsAggregator struct {
	HealthCheckLatencies  *prometheus.HistogramVec
	ActiveProcesses       prometheus.Gauge
	StandbyProcesses      prometheus.Gauge
	LeadershipTransitions *prometheus.CounterVec
	ProcessFailures       prometheus.Counter
	LeasesAttained        prometheus.Counter
	LeasesLost            prometheus.Counter
	KeyChangesDetected    prometheus.Counter
	ActivationAttempts    prometheus.Counter
	StandbyAttempts       prometheus.Counter
	BlocksProduced        prometheus.Counter
	BlocksReceived        prometheus.Counter
}

// NewMetricsAggregator creates a new MetricsAggregator.
func NewMetricsAggregator() *MetricsAggregator {
	m := &MetricsAggregator{}

	m.ActiveProcesses = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "nodeos_monitor_active_processes",
		Help: "How many processes are active",
	},
	)
	prometheus.MustRegister(m.ActiveProcesses)

	m.StandbyProcesses = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "nodeos_monitor_standby_processes",
		Help: "How many processes are standby",
	},
	)
	prometheus.MustRegister(m.StandbyProcesses)

	m.ProcessFailures = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "nodeos_monitor_process_failures",
			Help: "How many times the underlying nodeos process failed",
		},
	)
	prometheus.MustRegister(m.ProcessFailures)

	m.LeadershipTransitions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nodeos_monitor_leadership_transitions",
			Help: "How many times the nodeos process transitions between active and standby states, and vice versa",
		},
		[]string{"previous_status", "new_status"},
	)
	prometheus.MustRegister(m.LeadershipTransitions)

	m.LeasesAttained = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "nodeos_monitor_leases_attained",
			Help: "How many times the nodeos process attained an Etcd lease",
		},
	)
	prometheus.MustRegister(m.LeasesAttained)

	m.LeasesLost = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "nodeos_monitor_leases_lost",
			Help: "How many times the nodeos process lost an Etcd lease",
		},
	)
	prometheus.MustRegister(m.LeasesLost)

	m.KeyChangesDetected = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "nodeos_monitor_key_changes_detected",
			Help: "How many times the internal watcher detected an Etcd key change",
		},
	)
	prometheus.MustRegister(m.KeyChangesDetected)

	m.ActivationAttempts = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "nodeos_monitor_activation_attempts",
			Help: "How many times we tried to mark the nodeos process as active",
		},
	)
	prometheus.MustRegister(m.ActivationAttempts)

	m.StandbyAttempts = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "nodeos_monitor_standby_attempts",
			Help: "How many times we tried to mark the nodeos process as standby",
		},
	)
	prometheus.MustRegister(m.StandbyAttempts)

	m.BlocksProduced = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "nodeos_monitor_blocks_produced",
			Help: "How many blocks we have produced",
		},
	)
	prometheus.MustRegister(m.BlocksProduced)

	m.BlocksReceived = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "nodeos_monitor_blocks_received",
			Help: "How many blocks we have received",
		},
	)
	prometheus.MustRegister(m.BlocksReceived)

	return m
}
