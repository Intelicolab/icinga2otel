package consumer

import (
	"context"
	"encoding/json"
	"bufio"
	"bytes"
	"github.com/mdetrano/icinga2otel/internal/config"
	"github.com/mdetrano/icinga2otel/internal/objectcache"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	apiLog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"io"
	"log/slog"
	"os"
	"time"
	"os/signal"
	"syscall"
	"sync"
	"crypto/tls"
	"crypto/x509"
	"google.golang.org/grpc/credentials"

)

//----

var (
	pool     chan int
	cMetrics chan metricdata.Metrics
	cMetricsDone chan bool
	loggerProvider   *log.LoggerProvider
	logger   apiLog.Logger
)

func init() {
	slog.Info("Initializing Consumer.")
	pool = make(chan int, config.Config.Workers)
	for i := range config.Config.Workers {
		pool <- i
	}
	cMetrics = make(chan metricdata.Metrics, config.Config.BatchSize)
	cMetricsDone = make(chan bool)

}

// Init Logger Provider
func init() {
	ctx := context.Background()

	var exporter log.Exporter
	var err error

	//config package makes sure this is set
	if os.Getenv("OTEL_EXPORTER_OTLP_LOGS_PROTOCOL") == "grpc" {
		slog.Info("Initialzing grpc logger.")
		if os.Getenv("OTEL_EXPORTER_OTLP_CERTIFICATE") != "" || os.Getenv("OTEL_EXPORTER_OTLP_LOGS_CERTIFICATE") != "" { // work around bug https://github.com/open-telemetry/opentelemetry-go/issues/6661
			creds := credentials.NewTLS(getTlsConfig())
			exporter, err = otlploggrpc.New(ctx, otlploggrpc.WithTLSCredentials(creds) )
		} else {
			exporter, err = otlploggrpc.New(ctx )
		}
	} else {
		slog.Info("Initialzing http logger.")
		exporter, err = otlploghttp.New(ctx)
	}

	if err != nil {
		slog.Error("Could not initialize logs exporter.", "error", err)
	}

	res, _ := resource.New(ctx,
		resource.WithFromEnv(),
	)

	loggerProvider = log.NewLoggerProvider(
		log.WithResource(res),
		log.WithProcessor(log.NewBatchProcessor(exporter)),
	)

	logger = loggerProvider.Logger(config.Config.LogScopeName)

}

func worker(id int, eventBytes []byte) {
	defer func() { pool <- id }() //return id to pool when done
	slog.Debug("Starting worker", "id", id)

	var event IcingaEvent
	json.Unmarshal(eventBytes, &event)

	switch event.Type {
	case "CheckResult":
		sendMetric(event)
	case "ObjectModified", "ObjectCreated":
		updateCache(event)
	default:
		sendLog(event)
	}
	slog.Debug("Ending worker", "id", id)

}

func sendLog(event IcingaEvent) {

	record := apiLog.Record{}
	ctx := context.Background()

	var msg string = event.LogMessage()
	var logAttrs []apiLog.KeyValue = event.OtelLogAttributes()

	eventMicro := int64(event.Timestamp * 1000000)
	logTime := time.UnixMicro(eventMicro)

	record.SetSeverity(event.StateOtelSeverity())
	record.SetBody(apiLog.StringValue(msg))
	record.AddAttributes(logAttrs...)
	record.SetTimestamp(logTime)

	logger.Emit(ctx, record)

	slog.Info("Sent Event Log", "event", msg)
}

func sendMetric(event IcingaEvent) {

	var pointAttributes []attribute.KeyValue = event.OtelMetricAttributes()

	eventMicro := int64(event.Timestamp * 1000000)
	metricTime := time.UnixMicro(eventMicro)

	for _, pdv := range event.CheckResult.PerformanceData {
		slog.Debug("Processing metric.", "label", pdv.Label)
		cMetrics <- pdv.GetOtelMetric(pointAttributes, metricTime)
	}
}

func batchMetrics(wg *sync.WaitGroup) {

	defer wg.Done()

	var sendMetric metricdata.Metrics
	batch := make([]metricdata.Metrics, 0, config.Config.BatchSize)
	var timedOut bool = false
	var shutdown bool = false

	ctx := context.Background()

	var exporter metric.Exporter
	var err error

	res, _ := resource.New(ctx,
		resource.WithFromEnv(),
	)

	//config package makes sure this is set
	if os.Getenv("OTEL_EXPORTER_OTLP_METRICS_PROTOCOL") == "grpc" {
		slog.Info("Initialzing grpc metric exporter.")
		exporter, err = otlpmetricgrpc.New(ctx)
	} else {
		slog.Info("Initialzing http metric exporter.")
		exporter, err = otlpmetrichttp.New(ctx)
	}

	if err != nil {
		slog.Error("Problem initializing metric exporter! %s", err)
	}

	timeout := time.After(config.Config.BatchTime)

	for {
		select {
		case sendMetric = <-cMetrics:
			batch = append(batch, sendMetric)
		case <-timeout:
			timeout = time.After(config.Config.BatchTime)
			timedOut = (len(batch) > 0)
		case <-cMetricsDone:
			slog.Info("Signal caught. Flushing metrics batch and terminating.")
			shutdown = true
		}

		if (len(batch) >= config.Config.BatchSize) || timedOut || shutdown {
			slog.Debug("Sending Metrics Batch.")

			rm := metricdata.ResourceMetrics{
				Resource: res,
				ScopeMetrics: []metricdata.ScopeMetrics{
					{
						Scope:   instrumentation.Scope{Name: config.Config.MetricScopeName, Version: ""},
						Metrics: batch,
					},
				},
			}

			if err := exporter.Export(context.Background(), &rm); err != nil {
				slog.Error("Problem sending data!!", "error", err)
			}

			slog.Info("Metrics Batch Sent.", "count", len(batch))

			//reset
			batch = batch[:0]
			timeout = time.After(config.Config.BatchTime)
			timedOut = false

		}
		if shutdown {
			return
		}
	}

}

func updateCache(e IcingaEvent) {

	switch e.ObjectType {
	case "Service":
		objectcache.RefreshServices(e.ObjectName)
	case "Host":
		objectcache.RefreshHosts(e.ObjectName)
	}
}


// need to set this up manually for Logs exporter as workaround to  https://github.com/open-telemetry/opentelemetry-go/issues/6661
func getTlsConfig() (*tls.Config) {

	certPool := x509.NewCertPool()

	certFile := os.Getenv("OTEL_EXPORTER_OTLP_LOG_CERTIFICATE")
	if certFile == "" {
		certFile = os.Getenv("OTEL_EXPORTER_OTLP_CERTIFICATE")
	}

	if certFile != "" {
		if certPEM, err := os.ReadFile(certFile); err != nil {
			slog.Warn("Otel Exporter Certfile was specified but could not be read.","error", err)
		} else {
			if ok := certPool.AppendCertsFromPEM(certPEM); !ok {
				slog.Warn("Otel Exporter Certfile was specified but could not be added.")
			} else {
				slog.Info("Otel Exporter certificate added.")
			}
		}
	}

	tlsConfig := &tls.Config{
                RootCAs: certPool,
	}

	return tlsConfig
}

func Otel(reader *io.PipeReader) {

	var id int
	var wg sync.WaitGroup

	scanner := bufio.NewScanner(reader)

	cSignal := make(chan os.Signal, 1)
	signal.Notify(cSignal, syscall.SIGINT, syscall.SIGTERM)

	go func(){
		select {
		case <-cSignal:
			loggerProvider.ForceFlush(context.Background())
			cMetricsDone <- true
			wg.Wait()
			os.Exit(0)
		}
	}()

	wg.Add(1)
	go batchMetrics(&wg)

	for scanner.Scan() {
		eventBytes := bytes.Clone(scanner.Bytes()) //must Clone!
		id = <-pool //wait for an id from the pool
		go worker(id, eventBytes)
		slog.Debug("Scan worker fired", "bytes", len(eventBytes))
	}

}
