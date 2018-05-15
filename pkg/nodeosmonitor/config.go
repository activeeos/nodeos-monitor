package nodeosmonitor

// Config are all of the different options available for use by a
// monitor.
type Config struct {
	NodeosPath                string
	NodeosArgs                []string
	ActiveConfigTemplateFile  string
	StandbyConfigTemplateFile string
	LockClaimDelaySeconds     int64
	LockTTLSeconds            int64
	EtcdKey                   string
	EtcdCertFile              string
	EtcdKeyFile               string
	EtcdCAFile                string
}
