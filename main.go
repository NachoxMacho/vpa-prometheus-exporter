package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/codes"

	"github.com/grafana/pyroscope-go"
	pyroscope_pprof "github.com/grafana/pyroscope-go/http/pprof"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func gracefulShutdown(apiServer *http.Server, done chan bool) {
	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Listen for the interrupt signal.
	<-ctx.Done()

	slog.Info("shutting down gracefully, press Ctrl+C again to force")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := apiServer.Shutdown(ctx); err != nil {
		slog.Error("server shutdown failed", slog.String("error", err.Error()))
	}

	slog.Warn("server exiting")

	// Notify the main goroutine that the shutdown is complete
	done <- true
}

func main() {
	pyroscopeAddr := os.Getenv("PYROSCOPE_ADDR")

	pyro, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: "vpa-operator",
		ServerAddress:   pyroscopeAddr,
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileInuseSpace,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileGoroutines,
			pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration,
			pyroscope.ProfileBlockCount,
			pyroscope.ProfileBlockDuration,
		},
	})
	if err != nil {
		slog.Warn("failed to connect to profiler", slog.String("error", err.Error()))
	}
	defer func() {
		err := pyro.Stop()
		if err != nil {
			slog.Error("stopped profiling", slog.String("error", err.Error()))
		}
	}()

	traceAddr := os.Getenv("OTEL_ENDPOINT")
	if traceAddr != "" {
		ctx, shutdown, err := InitializeTracer(traceAddr)
		if err != nil {
			slog.Error("Failed to initialize tracer", slog.String("Error", err.Error()))
			os.Exit(1)
		}
		defer func() {
			err := shutdown(ctx)
			if err != nil {
				slog.Error("Failed to shutdown tracer", slog.String("Error", err.Error()))
				os.Exit(1)
			}
		}()
	}

	config, err := rest.InClusterConfig()
	if errors.Is(err, rest.ErrNotInCluster) {
		var kubeconfig *string
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
		} else {
			kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
		}
		flag.Parse()
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	}

	if err != nil {
		slog.Error("failed to connect to kubernetes cluster", slog.String("error", err.Error()))
		os.Exit(1)
	}

	vpaClient, err := versioned.NewForConfig(config)
	if err != nil {
		slog.Error("failed to build vpa client", slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.Info("Scanning for changes")

	StartMetricRecording("VPARecommendations", recordVPARecommendations(vpaClient), time.Minute*1)

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.HandleFunc("/debug/pprof/profile", pyroscope_pprof.Profile)

	mux.Handle("/metrics", promhttp.Handler())

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		_, span := StartTrace(context.TODO(), "readyz")
		defer span.End()
		w.WriteHeader(204)
		span.SetStatus(codes.Ok, "completed call")
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, span := StartTrace(context.TODO(), "healthz")
		defer span.End()
		w.WriteHeader(204)
		span.SetStatus(codes.Ok, "completed call")
	})

	server := http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// Create a done channel to signal when the shutdown is complete
	done := make(chan bool, 1)

	// Run graceful shutdown in a separate goroutine
	go gracefulShutdown(&server, done)

	slog.Info("Server starting", slog.String("url", server.Addr))

	err = server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(fmt.Sprintf("http server error: %s", err))
	}

	// Wait for the graceful shutdown to complete
	<-done

}
