package nodeosmonitor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"

	"code.cloudfoundry.org/clock"
	"github.com/coreos/etcd/clientv3"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Config are all of the different options available for use by a
// monitor.
type Config struct {
	DebugMode        bool
	NodeosPath       string
	NodeosArgs       []string
	ActiveConfigDir  string
	StandbyConfigDir string
	EtcdEndpoints    []string
	EtcdCertPath     string
	EtcdKeyPath      string
	EtcdCAPath       string
	FailoverGroup    string
}

func getEtcdClient(conf *Config) (*clientv3.Client, error) {
	var tlsCert *tls.Certificate
	if conf.EtcdCertPath != "" {
		cert, err := tls.LoadX509KeyPair(conf.EtcdCertPath, conf.EtcdKeyPath)
		if err != nil {
			return nil, errors.Wrapf(err, "error parsing Etcd client certificate")
		}
		tlsCert = &cert
	}

	var caPool *x509.CertPool
	if conf.EtcdCAPath != "" {
		caBytes, err := ioutil.ReadFile(conf.EtcdCAPath)
		if err != nil {
			return nil, errors.Wrapf(err, "error reading Etcd CA file")
		}

		caPool := x509.NewCertPool()
		ok := caPool.AppendCertsFromPEM(caBytes)
		if !ok {
			return nil, errors.New("error: failed to parse root certificate")
		}
	}

	tlsConf := &tls.Config{
		RootCAs: caPool,
	}
	if tlsCert != nil {
		tlsConf.Certificates = []tls.Certificate{*tlsCert}
	}

	client, err := clientv3.New(clientv3.Config{
		TLS:       tlsConf,
		Endpoints: conf.EtcdEndpoints,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error creating Etcd client")
	}

	return client, nil
}

// NodeosMonitor contains the services needed to run an EOS Nodeos
// Process monitor.
type NodeosMonitor struct {
	config   *Config
	failover *FailoverManager
}

// NewNodeosMonitor creates a new NodeosMonitor instance.
func NewNodeosMonitor(conf *Config) (*NodeosMonitor, error) {
	if conf.DebugMode {
		logrus.SetLevel(logrus.DebugLevel)
	}

	etcd, err := getEtcdClient(conf)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating Etcd client")
	}

	activeArgs := append(
		conf.NodeosArgs,
		"--config-dir", conf.ActiveConfigDir,
	)
	activeProcess := NewProcessMonitor(conf.NodeosPath, activeArgs)

	standbyArgs := append(
		conf.NodeosArgs,
		"--config-dir", conf.StandbyConfigDir,
	)
	standbyProcess := NewProcessMonitor(conf.NodeosPath, standbyArgs)

	failoverConfig := &FailoverConfig{
		ID:             uuid.New().String(),
		EtcdKey:        conf.FailoverGroup,
		Clock:          clock.NewClock(),
		LeaseClient:    etcd.Lease,
		WatcherClient:  etcd.Watcher,
		KVClient:       etcd.KV,
		ActiveProcess:  activeProcess,
		StandbyProcess: standbyProcess,
	}

	return &NodeosMonitor{
		config:   conf,
		failover: NewFailoverManager(failoverConfig),
	}, nil
}

// Start begins the Nodeos monitoring process. Start runs
// asynchronously and immediately returns.
func (n *NodeosMonitor) Start(ctx context.Context) {
	n.failover.Start(ctx)
	if err := n.failover.TryActivate(ctx); err != nil {
		logrus.WithError(err).Errorf("error attempting initial activation")
	}
}
