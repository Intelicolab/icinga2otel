package config

import (
	"log/slog"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"net"
	"os"
	"strings"
	"time"
	"fmt"
	"encoding/base64"
	"golang.org/x/net/http/httpguts"
)

type sConfig struct {
	IcingaHosts []string `mapstructure:"icinga_host"`
	IcingaUser  string `mapstructure:"icinga_user"`
	IcingaPass  string `mapstructure:"icinga_pass"`
	IcingaPassFile  string `mapstructure:"icinga_pass_file"`
	IcingaCert  string `mapstructure:"icinga_certificate"`
	IcingaClientCert  string `mapstructure:"icinga_client_certificate"`
	IcingaClientKey  string `mapstructure:"icinga_client_key"`
	IcingaInsecure     bool `mapstructure:"icinga_insecure"`
	IcingaInsecureHost     bool `mapstructure:"icinga_insecure_host"`
	IcingaEventTypes	[]string `mapstructure:"icinga_event_types"`
	IcingaFilter	string `mapstructure:"icinga_filter"`
	IcingaQueue	string `mapstructure:"icinga_queue"`
	LogScopeName	string `mapstructure:"log_scope_name"`
	MetricScopeName	string `mapstructure:"metric_scope_name"`
	LogAttrs	[]string `mapstructure:"log_attrs"`
	MetricAttrs	[]string `mapstructure:"metric_attrs"`
	Quiet     bool
	Debug       bool
	RetryDelay time.Duration `mapstructure:"retry_delay"`
	BatchSize int `mapstructure:"batch_size"`
	BatchTime time.Duration `mapstructure:"batch_time"`
	Workers int
	CollectorUser  string `mapstructure:"collector_user"`
	CollectorPass  string `mapstructure:"collector_pass"`
	CollectorPassFile  string `mapstructure:"collector_pass_file"`
	CollectorAuthType string `mapstructure:"collector_auth_type"`

}

var Config sConfig

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))) //set structured logging before anything else
	slog.Info("Reading Configuration.")

	var err error

	pflag.CommandLine.SetNormalizeFunc(func(f *pflag.FlagSet, name string) pflag.NormalizedName {
		return pflag.NormalizedName(strings.ToLower(name))
	})

	configFile := pflag.String("config", "", "Path to configuration file.")

	pflag.StringSlice("icinga_host", []string{"localhost"}, "Icinga Host.  Single host or comma separated list. Example: 'localhost:5665,otherhost:5665'.  Port 5665 will be added if not specified.")
	pflag.String("icinga_user", "", "Icinga API Username.")
	pflag.String("icinga_pass", "", "Icinga API Password.")
	pflag.String("icinga_pass_file", "", "Icinga API Password File.  Read pass from here unless explicitly set with icinga_pass")
	pflag.String("icinga_certificate", "", "Path to Icinga Certificate File for server validation.")
	pflag.String("icinga_client_certificate", "", "If authorizing API client with certificate, this is the path to the client certificate file")
	pflag.String("icinga_client_key", "", "If authorizing API client with certificate, this is the path to the client key file")
	pflag.Bool("icinga_insecure", false, "Skip certificate validation")
	pflag.Bool("icinga_insecure_host", false, "Skip only hostname validation, but validate certificate is signed.")
	pflag.StringSlice("icinga_event_types",[]string{"CheckResult","StateChange"},"Event Stream types to monitor. Comma separated.")
	pflag.String("icinga_filter", "", "Filter expression to use in the EventStream request. See icinga2-api-filters documentation. (ex. event.service==\"Ping\" ) ")
	pflag.String("icinga_queue", "icinga2otel", "Name for Icinga Queue in Event Stream request.")

        pflag.String("log_scope_name", "monitor.logger", "Scope Name for Logs")
        pflag.String("metric_scope_name", "monitor.metrics", "Scope Name for Metrics")
	pflag.StringSlice("log_attrs",[]string{},"List of object attributes to include in logs. (e.g. host.address,service.vars.SERVICE_TYPE) .")
	pflag.StringSlice("metric_attrs",[]string{},"List of object attributes to include in metrics. (e.g. host.address,service.vars.SERVICE_TYPE) .")

	pflag.String("otel_service_name", "monitor", "Open Telemetry's OTEL_SERVICE_NAME")
	pflag.String("otel_exporter_otlp_endpoint", "http://localhost:4317", "Open Telemetry's OTEL_EXPORTER_OTLP_ENDPOINT")
	pflag.String("otel_exporter_otlp_logs_endpoint", "", "Open Telemetry's OTEL_EXPORTER_OTLP_LOGS_ENDPOINT")
	pflag.String("otel_exporter_otlp_metrics_endpoint", "", "Open Telemetry's OTEL_EXPORTER_OTLP_METRICS_ENDPOINT")
	pflag.String("otel_exporter_otlp_protocol", "", "Open Telemetry's OTEL_EXPORTER_OTLP_PROTOCOL")
	pflag.String("otel_exporter_otlp_logs_protocol", "", "Open Telemetry's OTEL_EXPORTER_OTLP_LOGS_PROTOCOL")
	pflag.String("otel_exporter_otlp_metrics_protocol", "", "Open Telemetry's OTEL_EXPORTER_OTLP_METRICS_PROTOCOL")
	pflag.String("otel_exporter_otlp_compression", "", "Open Telemetry's OTEL_EXPORTER_OTLP_COMPRESSION (e.g. gzip)")
	pflag.String("otel_exporter_otlp_logs_compression", "", "Open Telemetry's OTEL_EXPORTER_OTLP_LOGS_COMPRESSION (e.g. none, gzip)")
	pflag.String("otel_exporter_otlp_metrics_compression", "", "Open Telemetry's OTEL_EXPORTER_OTLP_METRICS_COMPRESSION (e.g. none. gzip)")
	pflag.StringToString("otel_exporter_otlp_headers", map[string]string{}, "Open Telemetry's OTEL_EXPORTER_OTLP_HEADERS.")
	pflag.StringToString("otel_exporter_otlp_logs_headers", map[string]string{}, "Open Telemetry's OTEL_EXPORTER_OTLP_LOGS_HEADERS.")
	pflag.StringToString("otel_exporter_otlp_metrics_headers", map[string]string{}, "Open Telemetry's OTEL_EXPORTER_OTLP_METRICS_HEADERS.")
	pflag.String("otel_exporter_otlp_certificate", "", "Open Telemetry's OTEL_EXPORTER_OTLP_CERTIFICATE")
	pflag.String("otel_exporter_otlp_logs_certificate", "", "Open Telemetry's OTEL_EXPORTER_OTLP_LOGS_CERTIFICATE")
	pflag.String("otel_exporter_otlp_metrics_certificate", "", "Open Telemetry's OTEL_EXPORTER_OTLP_METRICS_CERTIFICATE")
	pflag.String("otel_exporter_otlp_client_certificate", "", "Open Telemetry's OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE")
	pflag.String("otel_exporter_otlp_logs_client_certificate", "", "Open Telemetry's OTEL_EXPORTER_OTLP_LOGS_CLIENT_CERTIFICATE")
	pflag.String("otel_exporter_otlp_metrics_client_certificate", "", "Open Telemetry's OTEL_EXPORTER_OTLP_METRICS_CLIENT_CERTIFICATE")
	pflag.String("otel_exporter_otlp_client_key", "", "Open Telemetry's OTEL_EXPORTER_OTLP_CLIENT_KEY")
	pflag.String("otel_exporter_otlp_logs_client_key", "", "Open Telemetry's OTEL_EXPORTER_OTLP_LOGS_CLIENT_KEY")
	pflag.String("otel_exporter_otlp_metrics_client_key", "", "Open Telemetry's OTEL_EXPORTER_OTLP_METRICS_CLIENT_KEY")
	
	pflag.String("collector_user", "", "User for Basic Auth with Open Telemetry collector.")
	pflag.String("collector_pass", "", "Password for Basic Auth with Open Telemetry colelctor.")
	pflag.String("collector_pass_file", "", "Password file for collector_pass.  Read password from here unless explicitly set with collector_pass")
	pflag.String("collector_auth_type", "", "To use a specific Authorization type, such as ApiKey or Bearer, set that type here and set the encoded credential value in collector_pass or collector_pass_file.")

	pflag.Duration("retry_delay", 10 * time.Second, "Delay between retries when connection is lost to monitor. (e.g., 10s, 1m)")
	pflag.Int("batch_size", 1000, "Maximum number of metrics to queue before sending to collector.")
	pflag.Duration("batch_time", 30  * time.Second, "Send metrics to collector after this durataion. (e.g., 30s, 1m)")
	pflag.Int("workers", 8, "Number of worker threads.")
	pflag.Bool("quiet", false, "Quiet output.")
	pflag.Bool("debug", false, "Debugging output.")

	pflag.Parse()
	reader := viper.New()
	reader.SetConfigName("config")

	reader.AddConfigPath(".")
	reader.AddConfigPath("./config")

	if *configFile != "" { //override config file
		reader.SetConfigFile(*configFile)
	}

	err = reader.ReadInConfig()
	if err != nil && *configFile != "" {
		slog.Warn("Config file was specified but not read.", "error", err)
	}

	reader.BindPFlags(pflag.CommandLine)
	reader.AutomaticEnv()

	reader.Unmarshal(&Config)

	fixIcingaPorts()

	Config.IcingaPass = strFromFile(Config.IcingaPassFile, Config.IcingaPass)
	Config.CollectorPass = strFromFile(Config.CollectorPassFile, Config.CollectorPass)

	otelSetAuthHeader(reader)

	otelToEnv(reader)

	setOtelProtocols()

	setLogLevel()


}

func strFromFile(fn string, bounce string) (string) {
	if bounce != "" || fn == "" {
		return bounce
	}

	if r, err := os.ReadFile(fn); err != nil {
		slog.Warn("File was specified for configuration but could not be read.", "file", fn)
	} else {
		return strings.Trim(string(r), "\n")
	}

	return bounce

}

func fixIcingaPorts() {

	for i, val := range Config.IcingaHosts {
		if _, _, err := net.SplitHostPort(val); err != nil {
			Config.IcingaHosts[i] = net.JoinHostPort(val, "5665")
		}
	}
}

//Adjust headers if authorization option was requested
func otelSetAuthHeader(reader *viper.Viper) {

	if Config.CollectorUser == "" && Config.CollectorAuthType == "" && Config.CollectorPass == "" {
		return
	}

	authType := Config.CollectorAuthType
	if authType == "" {
		authType = "Basic"
	}

	authSpec := Config.CollectorPass

	if Config.CollectorUser != "" && authType == "Basic" {
		userPass := Config.CollectorUser + ":" + Config.CollectorPass
		authSpec = base64.StdEncoding.EncodeToString([]byte(userPass))
	}

	headerVal := fmt.Sprintf("%s %s", authType, authSpec)

	if !httpguts.ValidHeaderFieldValue(headerVal) {
		slog.Error("Value for Authorization Header not valid. Skipping.")
	}

	m := reader.GetStringMapString("otel_exporter_otlp_headers")
	m["Authorization"] = headerVal
	reader.Set("otel_exporter_otlp_headers", m)

	if m = reader.GetStringMapString("otel_exporter_otlp_logs_headers"); len(m) > 0 {
		m["Authorization"] = headerVal
		reader.Set("otel_exporter_otlp_logs_headers", m)
	}

	if m = reader.GetStringMapString("otel_exporter_otlp_metrics_headers"); len(m) > 0 {
		m["Authorization"] = headerVal
		reader.Set("otel_exporter_otlp_metrics_headers", m)
	}

}

// push OTEL_ vars into environment, libraries will use that for configuration
func otelToEnv(reader *viper.Viper) {

	for key, val := range reader.AllSettings() {
		if strings.HasPrefix(strings.ToLower(key), "otel_") {
			switch val.(type) {
			case string:
				os.Setenv(strings.ToUpper(key), val.(string))
			case map[string]interface{}:
				var demap []string
				for mk, mv := range val.(map[string]interface{}) {
					demap = append(demap, fmt.Sprintf("%s=%s", mk, mv))
				}
				os.Setenv(strings.ToUpper(key), strings.Join(demap,","))
			case map[string]string:
				var demap []string
				for mk, mv := range val.(map[string]string) {
					demap = append(demap, fmt.Sprintf("%s=%s", mk, mv))
				}
				os.Setenv(strings.ToUpper(key), strings.Join(demap,","))

			}
		}
	}

}

// Make sure the OTEL protocols are populated so the exporter can be chosen correctly
func setOtelProtocols() {

	logsProto := os.Getenv("OTEL_EXPORTER_OTLP_LOGS_PROTOCOL")
	metricsProto := os.Getenv("OTEL_EXPORTER_OTLP_METRICS_PROTOCOL")

	if logsProto != "" && metricsProto != "" {
		return
	}

	var defProto string = "http/protobuf"
	proto := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	logsEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT")
	metricsEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT")

	if logsEndpoint == "" {
		logsEndpoint = endpoint
	}

	if metricsEndpoint == "" {
		metricsEndpoint = endpoint
	}

	if logsProto == "" {
		if proto != "" {
			logsProto = proto
		} else {
			if _, port, err := net.SplitHostPort(logsEndpoint); err != nil && port == "4317" {
				logsProto = "grpc"
			} else {
				logsProto = defProto
			}
		}
	}

	if metricsProto == "" {
		if proto != "" {
			metricsProto = proto
		} else {
			if _, port, err := net.SplitHostPort(metricsEndpoint); err != nil && port == "4317" {
				metricsProto = "grpc"
			} else {
				metricsProto = defProto
			}
		}
	}

	os.Setenv("OTEL_EXPORTER_OTLP_LOGS_PROTOCOL", logsProto)
	os.Setenv("OTEL_EXPORTER_OTLP_METRICS_PROTOCOL", metricsProto)

}

func setLogLevel() {

	lvl := slog.LevelInfo

	if (Config.Quiet) {
		lvl = slog.LevelWarn
	}
	if (Config.Debug) {
		lvl = slog.LevelDebug
	}

	handlerOpts := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: lvl,

	})

	slog.SetDefault(slog.New(handlerOpts))

}
