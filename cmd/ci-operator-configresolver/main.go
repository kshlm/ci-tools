package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	prowConfig "k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/pjutil"

	"github.com/openshift/ci-tools/pkg/config"
	"github.com/openshift/ci-tools/pkg/load"
)

const (
	orgQuery     = "org"
	repoQuery    = "repo"
	branchQuery  = "branch"
	variantQuery = "variant"
)

type options struct {
	configPath   string
	registryPath string
	logLevel     string
	address      string
	gracePeriod  time.Duration
	cycle        time.Duration
	validateOnly bool
	flatRegistry bool
}

type traceResponseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func (w *traceResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *traceResponseWriter) Write(data []byte) (int, error) {
	size, err := w.ResponseWriter.Write(data)
	w.size += size
	return size, err
}

var (
	configresolverMetrics = struct {
		httpRequestDuration *prometheus.HistogramVec
		httpResponseSize    *prometheus.HistogramVec
		errorRate           *prometheus.CounterVec
	}{
		httpRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "configresolver_http_request_duration_seconds",
				Help:    "http request duration in seconds",
				Buckets: []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2},
			},
			[]string{"status", "path"},
		),
		httpResponseSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "configresolver_http_response_size_bytes",
				Help:    "http response size in bytes",
				Buckets: []float64{256, 512, 1024, 2048, 4096, 6144, 8192, 10240, 12288},
			},
			[]string{"status", "path"},
		),
		errorRate: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "configresolver_error_rate",
				Help: "number of errors, sorted by label/type",
			},
			[]string{"error"},
		),
	}
)

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.configPath, "config", "", "Path to config dirs")
	fs.StringVar(&o.registryPath, "registry", "", "Path to registry dirs")
	fs.StringVar(&o.logLevel, "log-level", "info", "Level at which to log output.")
	fs.StringVar(&o.address, "address", ":8080", "Address to run server on")
	fs.DurationVar(&o.gracePeriod, "gracePeriod", time.Second*10, "Grace period for server shutdown")
	fs.DurationVar(&o.cycle, "cycle", time.Minute*2, "Cycle duration for config reload")
	fs.BoolVar(&o.validateOnly, "validate-only", false, "Load the config and registry, validate them and exit.")
	fs.BoolVar(&o.flatRegistry, "flat-registry", false, "Disable directory structure based registry validation")
	fs.Parse(os.Args[1:])
	return o
}

func validateOptions(o options) error {
	_, err := log.ParseLevel(o.logLevel)
	if err != nil {
		return fmt.Errorf("invalid --log-level: %v", err)
	}
	if o.cycle == 0 {
		return fmt.Errorf("invalid cycle: duration cannot equal 0")
	}
	if o.configPath == "" {
		return fmt.Errorf("--config is required")
	} else {
		if _, err := os.Stat(o.configPath); err != nil && os.IsNotExist(err) {
			return fmt.Errorf("--config points to a nonexistent directory: %v", err)
		}
	}
	if o.registryPath == "" {
		return fmt.Errorf("--registry is required")
	} else {
		if _, err := os.Stat(o.registryPath); err != nil && os.IsNotExist(err) {
			return fmt.Errorf("--registry points to a nonexistent directory: %v", err)
		}
	}
	if o.validateOnly && o.flatRegistry {
		return errors.New("--validate-only and --flat-registry flags cannot be set simultaneously")
	}
	return nil
}

func recordError(label string) {
	labels := prometheus.Labels{"error": label}
	configresolverMetrics.errorRate.With(labels).Inc()
}

func missingQuery(w http.ResponseWriter, field string) {
	recordError(fmt.Sprintf("query missing %s", field))
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, "%s query missing or incorrect", field)
}

func handleWithMetrics(h http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t := time.Now()
		// Initialize the status to 200 in case WriteHeader is not called
		trw := &traceResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		h(trw, r)
		latency := time.Since(t)
		labels := prometheus.Labels{"status": strconv.Itoa(trw.statusCode), "path": r.URL.EscapedPath()}
		configresolverMetrics.httpRequestDuration.With(labels).Observe(latency.Seconds())
		configresolverMetrics.httpResponseSize.With(labels).Observe(float64(trw.size))
		log.Debugf("Response Time: %.6f", latency.Seconds())
		log.Debugf("Response Size: %.0f", float64(trw.size))
	})
}

func genericHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(http.StatusText(http.StatusNotFound)))
	}
}

func resolveConfig(configAgent load.ConfigAgent, registryAgent load.RegistryAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(http.StatusNotImplemented)
			w.Write([]byte(http.StatusText(http.StatusNotImplemented)))
			return
		}
		org := r.URL.Query().Get(orgQuery)
		if org == "" {
			missingQuery(w, orgQuery)
			return
		}
		repo := r.URL.Query().Get(repoQuery)
		if repo == "" {
			missingQuery(w, repoQuery)
			return
		}
		branch := r.URL.Query().Get(branchQuery)
		if branch == "" {
			missingQuery(w, branchQuery)
			return
		}
		variant := r.URL.Query().Get(variantQuery)
		info := config.Info{
			Org:     org,
			Repo:    repo,
			Branch:  branch,
			Variant: variant,
		}

		config, err := configAgent.GetConfig(info)
		if err != nil {
			recordError("config not found")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "failed to get config: %v", err)
			log.WithError(err).Warning("failed to get config")
			return
		}
		config, err = registryAgent.ResolveConfig(config)
		if err != nil {
			recordError("failed to resolve config with registry")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "failed to resolve config with registry: %v", err)
			log.WithError(err).Warning("failed to resolve config with registry")
			return
		}
		jsonConfig, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			recordError("failed to marshal config")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "failed to marshal config to JSON: %v", err)
			log.WithError(err).Errorf("failed to marshal config to JSON")
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(jsonConfig)
	}
}

func getConfigGeneration(agent load.ConfigAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "%d", agent.GetGeneration())
	}
}

func getRegistryGeneration(agent load.RegistryAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "%d", agent.GetGeneration())
	}
}

func init() {
	prometheus.MustRegister(configresolverMetrics.httpRequestDuration)
	prometheus.MustRegister(configresolverMetrics.httpResponseSize)
	prometheus.MustRegister(configresolverMetrics.errorRate)
}

func main() {
	o := gatherOptions()
	err := validateOptions(o)
	if err != nil {
		log.Fatalf("invalid options: %v", err)
	}
	level, _ := log.ParseLevel(o.logLevel)
	log.SetLevel(level)
	health := pjutil.NewHealth()
	metrics.ExposeMetrics("ci-operator-configresolver", prowConfig.PushGateway{})

	configAgent, err := load.NewConfigAgent(o.configPath, o.cycle, configresolverMetrics.errorRate)
	if err != nil {
		log.Fatalf("Failed to get config agent: %v", err)
	}

	registryAgent, err := load.NewRegistryAgent(o.registryPath, o.cycle, configresolverMetrics.errorRate, o.flatRegistry)
	if err != nil {
		log.Fatalf("Failed to get registry agent: %v", err)
	}

	if o.validateOnly {
		os.Exit(0)
	}

	// add handler func for incorrect paths as well; can help with identifying errors/404s caused by incorrect paths
	http.HandleFunc("/", handleWithMetrics(genericHandler()))
	http.HandleFunc("/config", handleWithMetrics(resolveConfig(configAgent, registryAgent)))
	http.HandleFunc("/configGeneration", handleWithMetrics(getConfigGeneration(configAgent)))
	http.HandleFunc("/registryGeneration", handleWithMetrics(getRegistryGeneration(registryAgent)))
	interrupts.ListenAndServe(&http.Server{Addr: o.address}, o.gracePeriod)
	health.ServeReady()
	interrupts.WaitForGracefulShutdown()
}
