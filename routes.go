package main

import (
	"net/http"
	"net/http/pprof"

	pyroscope_pprof "github.com/grafana/pyroscope-go/http/pprof"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func addRoutes(mux *http.ServeMux) {

	mux.HandleFunc("/debug/pprof", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.HandleFunc("/debug/pprof/profile", pyroscope_pprof.Profile)

	mux.Handle("/metrics", promhttp.Handler())

	mux.Handle("/ready", handleReady())
	mux.Handle("/health", handleHealth())
}
