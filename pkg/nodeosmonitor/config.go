package nodeosmonitor

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
