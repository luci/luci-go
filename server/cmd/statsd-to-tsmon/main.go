// Copyright 2020 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Executable statsd-to-tsmon implements a statsd sink that sends aggregated
// metrics to tsmon.
//
// Supports only integer counters, gauges and timers. Timers are converted into
// tsmon histograms.
//
// See https://github.com/b/statsd_spec for a reference.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"

	"go.chromium.org/luci/server"
)

var (
	statsdMetricsProcessed = metric.NewCounter(
		"luci/statsd/metrics_processed",
		"How many statsd metrics were processed (per outcome).",
		nil,
		field.String("outcome"), // see processStatsdPacket
	)
)

func main() {
	statsdPort := flag.Int(
		"statsd-port",
		8125,
		"Localhost UDP port to bind to.")
	configFile := flag.String(
		"statsd-to-tsmon-config",
		"/etc/statsd-to-tsmon/config.cfg",
		"Path to the config file.")

	opts := &server.Options{
		HTTPAddr: "-", // not serving any HTTP routes
	}

	server.Main(opts, nil, func(srv *server.Server) error {
		if *configFile == "" {
			return errors.New("-statsd-to-tsmon-config is required")
		}
		cfg, err := LoadConfig(*configFile)
		if err != nil {
			return errors.Fmt("failed to load the config file: %w", err)
		}

		// Statsd metrics are sent to an UDP port.
		pc, err := net.ListenPacket("udp", fmt.Sprintf("localhost:%d", *statsdPort))
		if err != nil {
			return errors.Fmt("failed to bind the UDP socket: %w", err)
		}

		// Spin in a loop, reading and processing incoming UDP packets.
		srv.RunInBackground("statsd", func(ctx context.Context) { mainLoop(ctx, pc, cfg, nil) })
		return nil
	})
}

func mainLoop(ctx context.Context, pc net.PacketConn, cfg *Config, tick chan struct{}) {
	go func() {
		<-ctx.Done()
		pc.Close()
	}()

	// Buffer to store incoming UDP packets in.
	buf := make([]byte, 32*1024)
	// Buffer to store parsed statds metrics.
	m := StatsdMetric{Name: make([][]byte, 0, 8)}
	// Number of consecutive UDP receive errors.
	errs := 0

	for ctx.Err() == nil {
		n, _, err := pc.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			errs += 1
			if errs > 1 {
				logging.Errorf(ctx, "%d consecutive errors in ReadFrom: %s\n", errs, err)
				clock.Sleep(ctx, time.Second) // do not spinlock on persistent errors
			} else {
				logging.Errorf(ctx, "Error in ReadFrom: %s\n", err)
			}
		} else {
			errs = 0
			processStatsdPacket(ctx, cfg, buf[:n], &m)
		}

		// Used for synchronization in tests.
		if tick != nil {
			select {
			case tick <- struct{}{}:
			case <-ctx.Done():
			}
		}
	}
}

func processStatsdPacket(ctx context.Context, cfg *Config, buf []byte, m *StatsdMetric) {
	// Counters to flush to statsdMetricsProcessed.
	var (
		countOK          int64
		countMalformed   int64
		countUnsupported int64
		countUnexpected  int64
		countSkipped     int64
		countUnknown     int64
	)

	for len(buf) != 0 {
		read, err := ParseStatsdMetric(buf, m)
		if err == nil {
			err = ConvertMetric(ctx, cfg, m)
		}
		switch err {
		case nil:
			countOK++
		case ErrMalformedStatsdLine:
			logging.Warningf(ctx, "Bad statsd line: %q", string(buf[:read]))
			countMalformed++
		case ErrUnsupportedType:
			logging.Warningf(ctx, "Unsupported metric type: %q", string(buf[:read]))
			countUnsupported++
		case ErrUnexpectedType:
			logging.Warningf(ctx, "Unexpected metric type: %q", string(buf[:read]))
			countUnexpected++
		case ErrSkipped: // this is expected, do not log
			countSkipped++
		default:
			logging.Warningf(ctx, "Error when processing %q: %s", string(buf[:read]), err)
			countUnknown++
		}
		buf = buf[read:]
	}

	if countOK != 0 {
		statsdMetricsProcessed.Add(ctx, countOK, "OK")
	}
	if countMalformed != 0 {
		statsdMetricsProcessed.Add(ctx, countMalformed, "MALFORMED")
	}
	if countUnsupported != 0 {
		statsdMetricsProcessed.Add(ctx, countUnsupported, "UNSUPPORTED")
	}
	if countUnexpected != 0 {
		statsdMetricsProcessed.Add(ctx, countUnexpected, "UNEXPECTED")
	}
	if countSkipped != 0 {
		statsdMetricsProcessed.Add(ctx, countSkipped, "SKIPPED")
	}
	if countUnknown != 0 {
		statsdMetricsProcessed.Add(ctx, countUnknown, "UNKNOWN")
	}
}
