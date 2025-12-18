# icinga2otel

This service facilitates transferring event data from an Icinga2 monitoring system to an OpenTelemetry or compatible collector.  It will
store logs and/or metric data from the monitor to the collector.  This is accomplished by subscribing to the EventStream API on the monitor and preparing
and sending the data as logs or metrics to the OpenTelemetry collecting system.  Filters may be applied to limit the data collected, and a configurable
list of attributes from the monitored Hosts and Services may be included with the data records.

## Requirements
Support for the Go language on system running icinga2otel

An Icinga2 monitor with an API user setup with the following permissions:
>  permissions = [ "objects/query/\*", "events/\*" ]

An OpenTelemetry collector (or system implementing the protocol).

## Installation

Obtain source files.  In the root directory of the source run:

>go mod download

(once, to obtain required dependencies for this project)

Optionally, compile an executable icinga2otel binary:
>go build .


## Usage

### Quickstart, insecure example
>go run . --icinga_insecure --icinga_host {ip address of icinga monitor} --icinga_user {icinga api user} --icinga_pass {password for icinga api user} --otel_exporter_otlp_endpoint {address and port of OpenTelemetry collector}

For full configuration options run:
>go run . --help


## Configuration

All configuration options may be passed as command line arguments, environment variables, or via config file.  The options are the same in all formats except must be capitalized when set
as environment variables (so the flag `--icinga_host` would become `ICINGA_HOST` as an environment variable).  A sample config file is included in config/config.toml.example which can be copied to config/config.toml and used as a starting point. The configuration options are processed via Go's Viper module so yaml and json formats for the configuration are also possible.

## Secure Options

Obtain the Icinga CA certificate, often located in /var/lib/icinga2/certs/ca.crt in the Icinga system.  Pass the path to that file in the `icinga_certificate` option.  If the address used to reach Icinga does not match the common name (CN) of its certificate, you can use `icinga_insecure_host` to bypass the hostname check (`icinga_insecure` will skip all certificate validation).

If using TLS communications to the OpenTelemetry collector, obtain the CA certificate for that system and pass the path to that file in the `otel_exporter_otlp_certificate` option.  If using separate systems for Logs and Metrics the certificates can be referenced in `otel_exporter_otlp_metrics_certificate` and `otel_exporter_otlp_logs_certificate`

If using HTTP client certificate authentication to the Icinga API, obtain the key and certificate files for the API user and use `icinga_client_key` and `icinga_client_certificate` to set the paths to those files.

If using HTTP client certificate authentication to the OpenTelemetry collector, obtain a client key and cert. for that system and set the paths to those in `otel_exporter_otlp_client_key` and `otel_exporter_otlp_client_certificate`.  Similarly to the CA certificate, options are available to set those certificate files separately for Logs and Metrics.

If using Basic Auth to the OpenTelemetry collector, the credentials may be passed in the `collector_user` and `collector_pass` options.  icinga2otel will add the Authorization header when these options are set.

The credentials for Icinga or the OpenTelemetry collector may be passed via file by specifying the path to the file in `icinga_pass_file` or `collector_pass_file`, and **not** configuring `icinga_pass` or `collector_pass` otherwise.

Additional authentication methods may be sepcified by using the option `collector_auth_type`.  In this case the value for the `collector_pass` (or from `collector_pass_file`) will be used as the authorization parameter.  icinga2otel will construct the Authorization header with that information.  For instance, setting `--collector_auth_type ApiKey` and `--collecor_pass tkn98765DCBA`  would set the "Authorization: ApiKey tkn9876DCBA" HTTP header when sending data to the OpenTelemetry collector system.

## Data and Performance Options

By default, icinga2otel will subscribe to the CheckResult and StateChange event streams on the Icinga system.  To customize the list of event types, use `icinga_event_types` option.  (Note: the system will always subscribe to the ObjectChanged, ObjectCreated streams in order to monitor changes for its internal object cache from which attributes for the OpenTelemetry system are drawn).   See the [Event Stream Documentation](https://icinga.com/docs/icinga-2/latest/doc/12-icinga2-api/#event-streams) for more details on what types are available.  The CheckResult stream is used to generate metric data.  All other types generate log data.

Use the `icinga_filter` option to filter which objects are included in the Event Stream.  Example: `--icinga_filter 'event.host == "icinga-master" && event.service == "Ping"'` . See [API Filter Documentation](https://icinga.com/docs/icinga-2/latest/doc/12-icinga2-api/#filters) for more info.

Use `metric_attrs` and `log_attrs` to set which object attributes are included in the records sent to the OpenTelemetry collector. Comma separated list.  Example: `host.address,service.vars.SERVICE_TYPE`

Set the `otel_exporter_otlp_compression` option to "gzip" to reduce network load between icinga2otel and the OpenTelemetry collector.  This can significantly increase throughput to the collector over slower networks.

The `batch_size` option controls how many metrics will be stored in memory before being flushed to the collector in a batch operation.

The `batch_time` option limits how much time metrics will be stored in memory before being flushed to the collector, regardless of whether `batch_size` was reached.

## Docker Container

A Dockerfile is included to build a container image.  To create:

>docker build . -t icinga2otel

## Acknowledgements

This project utilizes the following open-source libraries:

- **opentelemetry-go:** (License: Apache 2.0) [https://github.com/open-telemetry/opentelemetry-go]
- **spf13/viper:** (License: MIT) [https://github.com/spf13/viper]
- **spf13/pflag:** (License: BSD-3-Clause ) [https://github.com/spf13/pflag]

"Icinga" is a trademark of Icinga GmbH

"OpenTelemetry" is a trademark of The Linux Foundation.
