package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/activeeos/nodeos-monitor/pkg/nodeosmonitor"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var config nodeosmonitor.Config

var rootCmd = &cobra.Command{
	Use:   "nodeos-monitor",
	Short: "nodeos-monitor provides failover for EOS nodes",
	Run: func(cmd *cobra.Command, args []string) {
		if config.DebugMode {
			logrus.SetLevel(logrus.DebugLevel)
		}

		switch config.LogFormat {
		case "text":
		case "json":
			logrus.SetFormatter(&logrus.JSONFormatter{})
		default:
			logrus.Fatalf("unsupported log format %s", config.LogFormat)
		}

		ctx := context.Background()

		shutdown := make(chan os.Signal, 1)
		signal.Notify(shutdown, os.Interrupt, os.Kill, syscall.SIGTERM)

		monitor, err := nodeosmonitor.NewNodeosMonitor(&config)
		if err != nil {
			logrus.WithError(err).Fatalf("error starting monitor")
		}

		ctx, monitorCancel := context.WithCancel(ctx)
		defer monitorCancel()

		go monitor.Start(ctx)

		signal := <-shutdown
		logrus.Debugf("received shutdown signal: %v", signal)

		ctx, shutdownCancel := context.WithTimeout(ctx, 10*time.Second)
		defer shutdownCancel()

		// This will cancel all hanging Etcd requests. This is needed
		// because all requests need to finish for monitor.Shutdown to
		// finish. The requests only tend to hang when Etcd isn't
		// up. We don't have to execute monitorCancel immediately if
		// the system is functioning correctly because requests will
		// return quickly.
		go func() {
			time.Sleep(10 * time.Second)
			monitorCancel()
		}()

		monitor.Shutdown(ctx)
	},
}

func init() {
	rootCmd.Flags().StringVar(&config.NodeosPath, "nodeos", "/opt/eosio/bin/nodeos",
		"the path to the nodeos binary")
	rootCmd.Flags().StringArrayVar(&config.NodeosArgs, "nodeos-args", nil,
		"additional arguments to pass to nodeos")
	rootCmd.Flags().StringVar(&config.ActiveConfigDir, "active-config-dir", "/etc/nodeos-active-configs/",
		"the directory containing the configs for an active nodeos process")
	rootCmd.Flags().StringVar(&config.StandbyConfigDir, "standby-config-dir", "/etc/nodeos-standby-configs/",
		"the directory containing the configs for a standby nodeos process")
	rootCmd.Flags().StringArrayVar(&config.EtcdEndpoints, "etcd-endpoints", []string{"http://127.0.0.1:2379"},
		"the endpoints to Etcd")
	rootCmd.Flags().StringVar(&config.EtcdCertPath, "etcd-cert", "",
		"the Etcd client certificate")
	rootCmd.Flags().StringVar(&config.EtcdKeyPath, "etcd-key", "",
		"the Etcd client key")
	rootCmd.Flags().StringVar(&config.EtcdCAPath, "etcd-ca", "",
		"the Etcd CA to use")
	rootCmd.Flags().StringVar(&config.FailoverGroup, "failover-group", "eos",
		"the identifier for the group of nodes involved in the failover process")
	rootCmd.Flags().BoolVar(&config.DebugMode, "debug", false, "print debug logs")
	rootCmd.Flags().StringVar(&config.LogFormat, "log-format", "text", "log format (one of 'text' or 'json')")
	rootCmd.Flags().StringVar(&config.MetricsHTTPAddr, "metrics-addr", ":3000",
		"where to expose the HTTP Prometheus metrics")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
