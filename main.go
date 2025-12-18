package main

import (
	"log/slog"
	"io"
	"github.com/mdetrano/icinga2otel/internal/config"
	"github.com/mdetrano/icinga2otel/internal/producer"
	"github.com/mdetrano/icinga2otel/internal/consumer"
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

