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

package plugin

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

	"go.chromium.org/luci/cipd/client/cipd/plugin/protocol"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMain(m *testing.M) {
	isPluginProc := len(os.Args) >= 1 && strings.HasPrefix(os.Args[1], "PLUGIN_")
	if isPluginProc {
		ctx := context.Background()
		if err := pluginMain(ctx, os.Args[1]); err != nil {
			errors.Log(ctx, err)
			os.Exit(1)
		} else {
			os.Exit(0)
		}
	} else {
		os.Exit(m.Run())
	}
}

func pluginMain(ctx context.Context, mode string) error {
	if mode == "PLUGIN_CRASHING_ON_START" {
		os.Exit(2)
	}

	return Run(ctx, os.Stdin, func(ctx context.Context, conn *grpc.ClientConn) error {
		// Abuse "ResolveAdmission" as a notification channel that we have started.
		adm := protocol.NewAdmissionsClient(conn)
		adm.ResolveAdmission(ctx, &protocol.ResolveAdmissionRequest{
			AdmissionId: mode,
		})

		switch {
		case mode == "PLUGIN_WAIT":
			// "Run" will wait until the context is canceled by default.

		case mode == "PLUGIN_STUCK_IN_TERMINATION":
			<-ctx.Done()
			time.Sleep(30 * time.Second)

		case mode == "PLUGIN_CRASHING_AFTER_RPC":
			time.Sleep(100 * time.Millisecond)
			os.Exit(2)

		case mode == "PLUGIN_LOG_STUFF_AND_EXIT":
			logging.Infof(ctx, "Info")
			logging.Warningf(ctx, "Warning")
			logging.Errorf(ctx, "Error")
			os.Exit(0)

		case strings.HasPrefix(mode, "PLUGIN_NUM_"):
			// Just wait.

		default:
			return errors.Reason("unknown test mode %q", mode).Err()
		}
		return nil
	})
}

func launchPlugin(ctx context.Context, host *Host, mode string) (*Process, error) {
	fakeAdmissions := fakeAdmissionsServer{calls: make(chan string, 1)}

	proc, err := host.LaunchPlugin(ctx, []string{os.Args[0], mode}, &Controller{
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
			return nil, errors.Reason("unexpected RPC call %q != %q", msg, mode).Err()
		}
		return proc, nil
	}
}

func TestPlugins(t *testing.T) {
	t.Parallel()

	ctx := gologger.StdConfig.Use(context.Background())
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	Convey("With a host", t, func() {
		host := &Host{ServiceURL: "https://example.com"}
		defer host.Close(ctx)

		Convey("Plugin wrong command line", func() {
			_, err := host.LaunchPlugin(ctx, []string{"doesnt_exist"}, &Controller{})
			So(err, ShouldNotBeNil)
		})

		Convey("Plugin exits when host closes", func() {
			proc, err := launchPlugin(ctx, host, "PLUGIN_WAIT")
			So(err, ShouldBeNil)
			So(proc.Err(), ShouldBeNil)
			host.Close(ctx)
			So(proc.Err(), ShouldEqual, ErrTerminated)
		})

		Convey("Terminate works", func() {
			proc, err := launchPlugin(ctx, host, "PLUGIN_WAIT")
			So(err, ShouldBeNil)
			So(proc.Err(), ShouldBeNil)
			So(proc.Terminate(ctx), ShouldEqual, ErrTerminated)
			So(proc.Err(), ShouldEqual, ErrTerminated)
		})

		Convey("Respects Terminate timeout", func() {
			proc, err := launchPlugin(ctx, host, "PLUGIN_STUCK_IN_TERMINATION")
			So(err, ShouldBeNil)

			ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
			defer cancel()
			So(proc.Terminate(ctx), ShouldHaveSameTypeAs, &exec.ExitError{})
		})

		Convey("Plugin crashing on start", func() {
			_, err := launchPlugin(ctx, host, "PLUGIN_CRASHING_ON_START")
			// This is either an ExitError or a pipe error writing to stdin, depending
			// on how far LaunchPlugin progressed before the plugin process crashed.
			So(err, ShouldNotBeNil)
		})

		Convey("Plugin crashing after RPC", func() {
			proc, err := launchPlugin(ctx, host, "PLUGIN_CRASHING_AFTER_RPC")
			So(err, ShouldBeNil)
			select {
			case <-time.After(30 * time.Second):
				panic("the test is stuck")
			case <-proc.Done():
				So(proc.Err(), ShouldHaveSameTypeAs, &exec.ExitError{})
			}
		})

		Convey("Logging from plugin works", func() {
			ctx := memlogger.Use(ctx)
			proc, err := launchPlugin(ctx, host, "PLUGIN_LOG_STUFF_AND_EXIT")
			So(err, ShouldBeNil)
			select {
			case <-time.After(30 * time.Second):
				panic("the test is stuck")
			case <-proc.Done():
				So(proc.Err(), ShouldEqual, ErrTerminated)
			}

			msg := logging.Get(ctx).(*memlogger.MemLogger).Messages()
			So(msg, ShouldHaveLength, 3)
			So(msg[0].Level, ShouldEqual, logging.Info)
			So(msg[0].Msg, ShouldEndWith, "Info")
			So(msg[1].Level, ShouldEqual, logging.Warning)
			So(msg[1].Msg, ShouldEndWith, "Warning")
			So(msg[2].Level, ShouldEqual, logging.Error)
			So(msg[2].Msg, ShouldEndWith, "Error")
		})

		Convey("Multiple plugins", func() {
			proc := make([]*Process, 5)
			wg := sync.WaitGroup{}
			for i := 0; i < len(proc); i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					proc[i], _ = launchPlugin(ctx, host, fmt.Sprintf("PLUGIN_NUM_%d", i))
				}(i)
			}
			wg.Wait()

			// All are running.
			for _, p := range proc {
				So(p, ShouldNotBeNil)
				So(p.Err(), ShouldBeNil)
			}

			host.Close(ctx)

			// All are stopped.
			for _, p := range proc {
				So(p.Err(), ShouldEqual, ErrTerminated)
			}
		})

		Convey("Serving error", func() {
			host.testServeErr = fmt.Errorf("simulated grpc startup error")
			_, err := launchPlugin(ctx, host, "PLUGIN_WAIT")
			So(err, ShouldNotBeNil)
			_, err = launchPlugin(ctx, host, "PLUGIN_WAIT")
			So(err, ShouldNotBeNil)
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
