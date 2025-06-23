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

package host

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipd/client/cipd/plugin"
	"go.chromium.org/luci/cipd/client/cipd/plugin/plugins"
	"go.chromium.org/luci/cipd/client/cipd/plugin/protocol"
)

func init() {
	registerPluginMain("PLUGIN_BASIC", func(ctx context.Context, mode string) error {
		if mode == "CRASHING_ON_START" {
			os.Exit(2)
		}

		return plugins.Run(ctx, os.Stdin, func(ctx context.Context, conn *grpc.ClientConn) error {
			// Abuse "ResolveAdmission" as a notification channel that we have started.
			adm := protocol.NewAdmissionsClient(conn)
			adm.ResolveAdmission(ctx, &protocol.ResolveAdmissionRequest{
				AdmissionId: mode,
			})

			switch {
			case mode == "WAIT":
				// "Run" will wait until the context is canceled by default.

			case mode == "STUCK_IN_TERMINATION":
				<-ctx.Done()
				time.Sleep(30 * time.Second)

			case mode == "CRASHING_AFTER_RPC":
				time.Sleep(100 * time.Millisecond)
				os.Exit(2)

			case mode == "LOG_STUFF_AND_EXIT":
				logging.Infof(ctx, "Info")
				logging.Warningf(ctx, "Warning")
				logging.Errorf(ctx, "Error")
				os.Exit(0)

			case strings.HasPrefix(mode, "NUM_"):
				// Just wait.

			default:
				return errors.Fmt("unknown test mode %q", mode)
			}
			return nil
		})
	})
}

func launchPlugin(ctx context.Context, host *Host, mode string) (*PluginProcess, error) {
	fakeAdmissions := fakeAdmissionsServer{calls: make(chan string, 1)}

	proc, err := host.LaunchPlugin(ctx, []string{os.Args[0], "PLUGIN_BASIC", mode}, &Controller{
		Admissions: &fakeAdmissions,
	})
	if err != nil {
		return nil, err
	}

	// Wait until the plugin calls an RPC to make sure it is connected.
	select {
	case <-time.After(30 * time.Second):
		panic("the test is stuck")
	case <-proc.Done():
		return nil, proc.Err()
	case msg := <-fakeAdmissions.calls:
		if msg != mode {
			return nil, errors.Fmt("unexpected RPC call %q != %q", msg, mode)
		}
		return proc, nil
	}
}

func TestPlugins(t *testing.T) {
	t.Parallel()

	ctx := gologger.StdConfig.Use(context.Background())
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ftt.Run("With a host", t, func(t *ftt.Test) {
		host := &Host{}
		host.Initialize(plugin.Config{ServiceURL: "https://example.com"})
		defer host.Close(ctx)

		t.Run("Plugin wrong command line", func(t *ftt.Test) {
			_, err := host.LaunchPlugin(ctx, []string{"doesnt_exist"}, &Controller{})
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Plugin exits when host closes", func(t *ftt.Test) {
			proc, err := launchPlugin(ctx, host, "WAIT")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, proc.Err(), should.BeNil)
			host.Close(ctx)
			assert.Loosely(t, proc.Err(), should.Equal(ErrTerminated))
		})

		t.Run("Terminate works", func(t *ftt.Test) {
			proc, err := launchPlugin(ctx, host, "WAIT")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, proc.Err(), should.BeNil)
			assert.Loosely(t, proc.Terminate(ctx), should.Equal(ErrTerminated))
			assert.Loosely(t, proc.Err(), should.Equal(ErrTerminated))
		})

		t.Run("Respects Terminate timeout", func(t *ftt.Test) {
			proc, err := launchPlugin(ctx, host, "STUCK_IN_TERMINATION")
			assert.Loosely(t, err, should.BeNil)

			ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
			defer cancel()
			assert.Loosely(t, proc.Terminate(ctx), should.HaveType[*exec.ExitError])
		})

		t.Run("Plugin crashing on start", func(t *ftt.Test) {
			_, err := launchPlugin(ctx, host, "CRASHING_ON_START")
			// This is either an ExitError or a pipe error writing to stdin, depending
			// on how far LaunchPlugin progressed before the plugin process crashed.
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Plugin crashing after RPC", func(t *ftt.Test) {
			proc, err := launchPlugin(ctx, host, "CRASHING_AFTER_RPC")
			assert.Loosely(t, err, should.BeNil)
			select {
			case <-time.After(30 * time.Second):
				panic("the test is stuck")
			case <-proc.Done():
				assert.Loosely(t, proc.Terminate(ctx), should.HaveType[*exec.ExitError])
			}
		})

		t.Run("Logging from plugin works", func(t *ftt.Test) {
			ctx := memlogger.Use(ctx)
			proc, err := launchPlugin(ctx, host, "LOG_STUFF_AND_EXIT")
			assert.Loosely(t, err, should.BeNil)
			select {
			case <-time.After(30 * time.Second):
				panic("the test is stuck")
			case <-proc.Done():
				assert.Loosely(t, proc.Err(), should.Equal(ErrTerminated))
			}

			msg := logging.Get(ctx).(*memlogger.MemLogger).Messages()
			assert.Loosely(t, msg, should.HaveLength(3))
			assert.Loosely(t, msg[0].Level, should.Equal(logging.Info))
			assert.Loosely(t, msg[0].Msg, should.HaveSuffix("Info"))
			assert.Loosely(t, msg[1].Level, should.Equal(logging.Warning))
			assert.Loosely(t, msg[1].Msg, should.HaveSuffix("Warning"))
			assert.Loosely(t, msg[2].Level, should.Equal(logging.Error))
			assert.Loosely(t, msg[2].Msg, should.HaveSuffix("Error"))
		})

		t.Run("Multiple plugins", func(t *ftt.Test) {
			proc := make([]*PluginProcess, 5)
			wg := sync.WaitGroup{}
			for i := 0; i < len(proc); i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					proc[i], _ = launchPlugin(ctx, host, fmt.Sprintf("NUM_%d", i))
				}(i)
			}
			wg.Wait()

			// All are running.
			for _, p := range proc {
				assert.Loosely(t, p, should.NotBeNil)
				assert.Loosely(t, p.Err(), should.BeNil)
			}

			host.Close(ctx)

			// All are stopped.
			for _, p := range proc {
				assert.Loosely(t, p.Err(), should.Equal(ErrTerminated))
			}
		})

		t.Run("Serving error", func(t *ftt.Test) {
			host.testServeErr = fmt.Errorf("simulated grpc startup error")
			_, err := launchPlugin(ctx, host, "WAIT")
			assert.Loosely(t, err, should.NotBeNil)
			_, err = launchPlugin(ctx, host, "WAIT")
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}

type fakeAdmissionsServer struct {
	protocol.UnimplementedAdmissionsServer
	calls chan string
}

func (s *fakeAdmissionsServer) ResolveAdmission(ctx context.Context, req *protocol.ResolveAdmissionRequest) (*emptypb.Empty, error) {
	s.calls <- req.AdmissionId
	return &emptypb.Empty{}, nil
}
