package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

// --- Prometheus metrics ---
var (
	httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests",
	}, []string{"path", "method", "status"})

	httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"path"})
)

func init() {
	prometheus.MustRegister(httpRequests, httpDuration)
}

// --- OpenTelemetry / Jaeger setup ---
func initTracer() func() {
	jaegerEndpoint := os.Getenv("JAEGER_ENDPOINT")
	if jaegerEndpoint == "" {
		jaegerEndpoint = "http://jaeger:14268/api/traces"
	}

	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(jaegerEndpoint)))
	if err != nil {
		log.Printf("Warning: could not init Jaeger exporter: %v", err)
		return func() {}
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("self-healing-app"),
		)),
	)
	otel.SetTracerProvider(tp)
	return func() { _ = tp.Shutdown(nil) }
}

// --- Middleware: metrics + tracing ---
func instrument(path string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tracer := otel.Tracer("self-healing-app")
		ctx, span := tracer.Start(r.Context(), path)
		defer span.End()
		r = r.WithContext(ctx)

		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: 200}
		next(rw, r)
		duration := time.Since(start).Seconds()

		httpRequests.WithLabelValues(path, r.Method, http.StatusText(rw.status)).Inc()
		httpDuration.WithLabelValues(path).Observe(duration)
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// --- Handlers ---
func rootHandler(w http.ResponseWriter, r *http.Request) {
	serviceName := os.Getenv("SERVICE_NAME")
	if serviceName == "" {
		serviceName = "app-a"
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": serviceName,
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

// crashHandler is intentionally broken — used to demo K8s self-healing.
// Calling GET /crash causes the process to exit with code 1.
func crashHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("CRASH ENDPOINT HIT — exiting to demonstrate self-healing")
	os.Exit(1)
}

func main() {
	shutdown := initTracer()
	defer shutdown()

	mux := http.NewServeMux()
	mux.HandleFunc("/", instrument("/", rootHandler))
	mux.HandleFunc("/health", instrument("/health", healthHandler))
	mux.HandleFunc("/ready", instrument("/ready", readyHandler))
	mux.HandleFunc("/crash", instrument("/crash", crashHandler))
	mux.Handle("/metrics", promhttp.Handler())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Self-healing app starting on port %s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
