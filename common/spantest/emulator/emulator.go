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
	"os"
	"os/exec"
	"runtime"
	"time"

	spanins "cloud.google.com/go/spanner/admin/instance/apiv1"
	inspb "cloud.google.com/go/spanner/admin/instance/apiv1/instancepb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

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
					return nil, errors.Fmt(`neither "docker", nor "podman" are in PATH: `+
						`a containerization engine is required to run the Cloud Spanner emulator on %s`,
						runtime.GOOS)

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
		err = errors.Fmt("unrecognized CLOUD_SPANNER_EMULATOR_DRIVER %q", driver)
	}
	if err != nil {
		return
	}

	// This is mostly useful for the container code path, since startAsContainer
	// returns as soon as the container is up, but the Cloud Spanner emulator
	// inside of it might still be initializing itself. It should be extremely
	// quick for "gcloud" code path (and won't hurt as a double check).
	if err = waitAvailable(ctx, emu.grpcAddr, emu.ClientOptions()); err != nil {
		err = errors.Fmt("Cloud Spanner emulator readiness check failed: %w", err)
		return
	}

	done = true
	return
}

// GrpcAddr is the "127.0.0.1:<port>" address with the Cloud Spanner gRPC port.
func (e *Emulator) GrpcAddr() string {
	return e.grpcAddr
}

// ClientOptions returns a bundle of options to pass to the Cloud Spanner client
// to tell it to connect to the emulator.
func (e *Emulator) ClientOptions() []option.ClientOption {
	return []option.ClientOption{
		option.WithEndpoint(e.grpcAddr),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	}
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

// waitAvailable probes emulator's gRPC server, retrying a bunch of times.
func waitAvailable(ctx context.Context, addr string, opts []option.ClientOption) error {
	probeOnce := func() bool {
		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()

		client, err := spanins.NewInstanceAdminClient(ctx, opts...)
		if err != nil {
			return false
		}
		defer func() { _ = client.Close() }()

		_, err = client.GetInstance(ctx, &inspb.GetInstanceRequest{
			Name: "projects/ignored/instances/ignored",
		})
		switch status.Code(err) {
		case codes.OK, codes.NotFound:
			return true
		default:
			return false
		}
	}

	const maxAttempts = 20

	attempt := 0
	for ctx.Err() == nil {
		attempt++
		if probeOnce() {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt == maxAttempts {
			return errors.Fmt("can't connect to the emulator at %q after many attempts", addr)
		}
		clock.Sleep(ctx, 10*time.Millisecond*time.Duration(attempt))
	}

	return ctx.Err()
}
