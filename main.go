package main

import (
	"net/http"
	"os"

	netatmo "github.com/exzz/netatmo-api-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

var (
	log = &logrus.Logger{
		Out: os.Stderr,
		Formatter: &logrus.TextFormatter{
			DisableTimestamp: true,
		},
		Level:        logrus.InfoLevel,
		ExitFunc:     os.Exit,
		ReportCaller: false,
	}
)

func main() {
	cfg, err := parseConfig(os.Args, os.Getenv)
	if err != nil {
		log.Fatalf("Error in configuration: %s", err)
	}
	log.SetLevel(logrus.Level(cfg.LogLevel))

	log.Infof("Login as %s", cfg.Netatmo.Username)
	client, err := netatmo.NewClient(cfg.Netatmo)
	if err != nil {
		log.Fatalf("Error creating client: %s", err)
	}

	metrics := &netatmoCollector{
		log:             log,
		client:          client,
		refreshInterval: cfg.RefreshInterval,
		staleThreshold:  cfg.StaleDuration,
	}
	prometheus.MustRegister(metrics)

	http.Handle("/metrics", promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{}))
	http.Handle("/version", versionHandler(log))
	http.Handle("/", http.RedirectHandler("/metrics", http.StatusFound))

	log.Infof("Listen on %s...", cfg.Addr)
	log.Fatal(http.ListenAndServe(cfg.Addr, nil))
}
