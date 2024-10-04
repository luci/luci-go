// Copyright 2024 The LUCI Authors.
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

// Package emulator knows how to launch Cloud Spanner emulator.
//
// It uses a bunch of strategies to run it, depending on the host environment
// and `CLOUD_SPANNER_EMULATOR_DRIVER` env var.
//
// On Linux by default it would launch the emulator binary directly, by finding
// it through `gcloud`.
//
// On other OSes it will run it as a container either through Docker (default)
// or Podman (if `docker` is absent).
//
// In all cases `CLOUD_SPANNER_EMULATOR_DRIVER` can be used to override the
// default behavior:
//   - "gcloud": try to find the emulator binary on the host via `gcloud`.
//   - "docker": use `docker run ...` to run the container with the emulator.
//   - "podman": use `podman run ...` to run the container with the emulator.
//
// The emulator's gRPC socket will be bound to an available localhost port
// (different each time, picked by the OS), which is useful when running many
// tests in parallel.
//
// Only the gRPC port is exposed. The REST port is not exposed (and as
// a consequence, emulators started this way can't be accessed via gcloud
// spanner subcommands): unfortunately, when using "gcloud" method, there's no
// way to simultaneously support the REST port and have it be bound to an
// available TCP port. Looks like the REST port is exposed by some `gateway`
// binary, not by the spanner emulator binary itself. This is orchestrated by
// "gcloud emulators spanner start" command that launches the actual emulator
// and also the gRPC => REST gateway. Unlike the actual spanner emulator, this
// wrapper **requires** ports to be given in advance. There were attempts to
// workaround that by attempting to pick the random port in advance and then
// "very quickly" pass it to "gcloud emulators spanner start". This approach
// was unreliable and caused lots of flakiness in the tests.
//
// Note that when running the emulator as a container, this is not a problem,
// since the host port assignment is done by the containerization engine and it
// doesn't care that ports inside the container are predefined. But for
// compatibility with "gcloud" driver (the primary one), the REST port is not
// exposed at all.
//
// Since nothing in the tests seems to rely on the REST port (the Go Cloud
// Spanner library uses gRPC API exclusively), it's absence is not a big issue
// in practice.
package emulator

import (
	"bufio"
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// Emulator is a running Cloud Spanner emulator.
type Emulator struct {
	grpcAddr string
	stop     func()

	logDone  chan struct{}
	logWrite *io.PipeWriter
}

// Start starts a new empty Cloud Spanner emulator instance.
//
// Its gRPC serving port will be bound to some OS-assigned localhost port.
// The exact endpoint address will be available via [GrpcAddr].
//
// Emulator's output (both stdout and stderr) is logged into the active LUCI
// logger in the context at Info severity.
func Start(ctx context.Context) (emu *Emulator, err error) {
	driver := os.Getenv("CLOUD_SPANNER_EMULATOR_DRIVER")
	if driver == "" {
		if runtime.GOOS == "linux" {
			driver = "gcloud"
		} else {
			driver = "docker"
			if _, err := exec.LookPath(driver); err != nil {
				driver = "podman"
				if _, err := exec.LookPath(driver); err != nil {
					return nil, errors.Reason(
						`neither "docker", nor "podman" are in PATH: `+
							`a containerization engine is required to run the Cloud Spanner emulator on %s`,
						runtime.GOOS,
					).Err()
				}
			}
		}
	}

	logRead, logWrite := io.Pipe()
	emu = &Emulator{
		logDone:  make(chan struct{}),
		logWrite: logWrite,
	}

	go func() {
		defer close(emu.logDone)
		scanner := bufio.NewScanner(logRead)
		for scanner.Scan() {
			logging.Infof(ctx, "%s", scanner.Text())
		}
	}()

	done := false
	defer func() {
		if err != nil || !done {
			emu.Stop() // shuts down the log pipe
			emu = nil  // return nothing on errors
		}
	}()

	switch driver {
	case "gcloud":
		emu.grpcAddr, emu.stop, err = startViaGcloud(ctx, emu.logWrite)
	case "docker", "podman":
		emu.grpcAddr, emu.stop, err = startAsContainer(ctx, driver, emu.logWrite)
	default:
		err = errors.Reason("unrecognized CLOUD_SPANNER_EMULATOR_DRIVER %q", driver).Err()
	}
	if err != nil {
		return
	}

	// This is mostly useful for the container code path, since startAsContainer
	// returns as soon as the container is up, but the Cloud Spanner emulator
	// inside of it might still be initializing itself. It should be extremely
	// quick for "gcloud" code path (and won't hurt as a double check).
	if err = waitTCPPort(ctx, emu.grpcAddr); err != nil {
		err = errors.Annotate(err, "Cloud Spanner emulator readiness check failed").Err()
		return
	}

	done = true
	return
}

// GrpcAddr is the "127.0.0.1:<port>" address with the Cloud Spanner gRPC port.
func (e *Emulator) GrpcAddr() string {
	return e.grpcAddr
}

// Stop terminates the emulator process, wiping its state.
func (e *Emulator) Stop() {
	// If have the emulator running, terminate it and wait for it to stop. This
	// will also close the log pipe.
	if e.stop != nil {
		e.stop()
	}
	// Close the log pipe again, in case the emulator was not running. This does
	// nothing if the pipe was already closed.
	_ = e.logWrite.Close()
	// Wait for the log pipe goroutine to exit, don't want to leak it.
	<-e.logDone
}

// waitTCPPort tries to open a TCP connection, retrying a bunch of times.
func waitTCPPort(ctx context.Context, addr string) error {
	probeTCP := func() bool {
		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}

	const maxAttempts = 70

	attempt := 0
	for ctx.Err() == nil {
		attempt++
		if probeTCP() {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt == maxAttempts {
			return errors.Reason("can't connect to TCP endpoint %q after many attempts", addr).Err()
		}
		clock.Sleep(ctx, 10*time.Millisecond*time.Duration(attempt))
	}

	return ctx.Err()
}
