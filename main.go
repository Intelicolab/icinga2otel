package main

import (
	"log/slog"
	"io"
	"gms/icinga2otel/internal/config"
	"gms/icinga2otel/internal/producer"
	"gms/icinga2otel/internal/consumer"
)

func main() {
	slog.Info("Starting Up...")
	if config.Config.Quiet {
		slog.Warn("Starting up in quiet mode...")
	}

	reader, writer := io.Pipe()
	go producer.Icinga(writer)
	consumer.Otel(reader)

}

