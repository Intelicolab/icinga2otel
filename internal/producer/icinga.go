package producer

import (
	"github.com/Intelicolab/icinga2otel/internal/config"
	"github.com/Intelicolab/icinga2otel/internal/client"
	"github.com/Intelicolab/icinga2otel/internal/objectcache"
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"io"
	"net/http"
	"time"
)

var (
	errStream        = errors.New("Could not connect to Icinga EventStream")
)

type EventRequest struct {
	Queue string   `json:"queue"`
	Types []string `json:"types"`
	Filter string  `json:"filter"`
}

func connectScanner() (*bufio.Scanner, error) {

	url := fmt.Sprintf("https://%s/v1/events", client.GetIcingaHost())

	client := client.GetIcingaHttpClient()
	client.Timeout = 0

	// always monitor object changes to update the cache
	eventTypes := append(config.Config.IcingaEventTypes, "ObjectModified","ObjectCreated")

	req_data := &EventRequest{
		Queue: config.Config.IcingaQueue,
		Types: eventTypes,
		Filter: config.Config.IcingaFilter,
	}

	jreq_data, _ := json.Marshal(req_data)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jreq_data))
	if err != nil {
		return nil, fmt.Errorf("%w %w", errStream, err)
	}

	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(config.Config.IcingaUser, config.Config.IcingaPass)

	resp, err := client.Do(req)

	if err != nil {
		return nil, fmt.Errorf("%w %w", errStream, err)
	}

	if !(resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return nil, fmt.Errorf("%w Bad response code: %d", errStream, resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)

	// full refesh object cache on connect or reconnect
	go objectcache.Refresh()


	return scanner, nil

}

// Main Producer Loop
func Icinga(writer *io.PipeWriter) {

	defer writer.Close()

	slog.Info("Starting Event Producer.")

	//retries are instant at first, then after one seccond, then each retry_delay interval
	var attempt int = 0
	var pauses = [3]time.Duration{ time.Duration(0), time.Second, config.Config.RetryDelay }
	var pause = pauses[0]

	for {
		scanner, err := connectScanner()
		if err != nil {
			slog.Warn("Error connecting to Icinga API", "error", err)
		} else {

			for scanner.Scan() {
				slog.Debug("Producer Received", "line", string(scanner.Bytes()))
				writer.Write(scanner.Bytes())
				writer.Write([]byte("\n")) //consumer needs the delimiter, too
			}
			slog.Info("Event Stream Lost...")
		}

		if err == nil { attempt = 0 }

		if attempt < len(pauses) {
			pause = pauses[attempt]
		} else {
			pause = pauses[len(pauses) -1]
		}
		attempt++

		slog.Info("Scanner Exited.  Attempting Reconnect.","attempt", attempt,"pause", pause)
		time.Sleep(pause)
	}

}
