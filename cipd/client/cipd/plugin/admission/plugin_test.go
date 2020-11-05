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

package admission

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"

	"go.chromium.org/luci/cipd/client/cipd/plugin"
	"go.chromium.org/luci/cipd/client/cipd/plugin/protocol"
	"go.chromium.org/luci/cipd/common"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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
	switch mode {
	case "PLUGIN_NOT_CONNECTING":
		// Block until stdin closes (which indicates the host is closing us).
		io.Copy(ioutil.Discard, os.Stdin)
		return nil
	case "PLUGIN_CRASHING_WHEN_CONNECTING":
		os.Exit(2)
	}

	var count int32

	return RunPlugin(ctx, os.Stdin, "some version", func(ctx context.Context, req *protocol.Admission) error {
		cur := atomic.AddInt32(&count, 1)

		switch mode {
		case "PLUGIN_NORMAL_REPLY":
			if req.ServiceUrl == "https://good" {
				return nil
			}
			logging.Infof(ctx, "Rejecting %s:%s:%s", req.ServiceUrl, req.Package, common.ObjectRefToInstanceID(req.Instance))
			return status.Errorf(codes.FailedPrecondition, "the plugin says boo")

		case "PLUGIN_BLOCK_REQUEST":
			<-ctx.Done()
			return nil

		case "PLUGIN_CRASH_ON_SECOND_REQUEST":
			if cur == 2 {
				os.Exit(2)
			}
			return nil

		case "PLUGIN_BLOCK_ALL_BUT_FIRST":
			if cur != 1 {
				<-ctx.Done()
			}
			return nil

		default:
			return status.Errorf(codes.Aborted, "unknown mode")
		}
	})
}

func TestPlugin(t *testing.T) {
	t.Parallel()

	const testInstanceID = "qUiQTy8PR5uPgZdpSzAYSw0u0cHNKh7A-4XSmaGSpEcC"

	testPin := func(pkg string) common.Pin {
		return common.Pin{
			PackageName: pkg,
			InstanceID:  testInstanceID,
		}
	}

	ctx := gologger.StdConfig.Use(context.Background())
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	Convey("With a host", t, func() {
		host := &plugin.Host{}
		defer host.Close(ctx)

		Convey("Happy path", func() {
			plug := NewPlugin(ctx, host, []string{os.Args[0], "PLUGIN_NORMAL_REPLY"})
			defer plug.Close(ctx)

			good := plug.CheckAdmission("https://good", testPin("a/b"))
			bad := plug.CheckAdmission("https://bad", testPin("a/b"))

			// Reuses pending requests.
			dup := plug.CheckAdmission("https://good", testPin("a/b"))
			So(dup, ShouldEqual, good)

			// Wait until completion.
			So(good.Wait(ctx), ShouldBeNil)

			err := bad.Wait(ctx)
			So(err, ShouldHaveRPCCode, codes.FailedPrecondition)
			So(err, ShouldErrLike, "the plugin says boo")

			// Caches the result.
			So(bad.Wait(ctx), ShouldEqual, err)

			// Reuses finished requests.
			dup = plug.CheckAdmission("https://good", testPin("a/b"))
			So(dup, ShouldEqual, good)

			// "Forget" resolved promises.
			plug.ClearCache()

			// Makes a new one now.
			anotherGood := plug.CheckAdmission("https://good", testPin("a/b"))
			So(anotherGood, ShouldNotEqual, good)
			So(anotherGood.Wait(ctx), ShouldBeNil)

			// A bit of a stress testing.
			promises := make([]*Promise, 1000)
			for i := range promises {
				promises[i] = plug.CheckAdmission("https://good", testPin(fmt.Sprintf("pkg/%d", i)))
			}
			for _, p := range promises {
				if err := p.Wait(ctx); err != nil {
					So(err, ShouldBeNil) // spam convey only on failures
				}
			}

			plug.Close(ctx)

			// Rejects all requests right away if closed.
			p := plug.CheckAdmission("https://good", testPin("a/b/c/d/e"))
			So(p.Wait(ctx), ShouldEqual, ErrAborted)
		})

		Convey("Closing right after starting", func() {
			plug := NewPlugin(ctx, host, []string{os.Args[0], "PLUGIN_BLOCK_REQUEST"})
			p := plug.CheckAdmission("https://url", testPin("a/b"))
			plug.Close(ctx)

			// The exact error depends on how far we progressed before the plugin
			// was closed. Either way it should NOT be a context deadline.
			err := p.Wait(ctx)
			So(err, ShouldNotBeNil)
			So(ctx.Err(), ShouldBeNil)
		})

		Convey("Plugin is not connecting", func() {
			plug := NewPlugin(ctx, host, []string{os.Args[0], "PLUGIN_NOT_CONNECTING"})
			plug.timeout = time.Second
			defer plug.Close(ctx)

			Convey("Timeout", func() {
				p := plug.CheckAdmission("https://url", testPin("a/b"))
				So(p.Wait(ctx), ShouldNotBeNil)
				So(ctx.Err(), ShouldBeNil)
			})

			Convey("Closing while waiting", func() {
				p := plug.CheckAdmission("https://url", testPin("a/b"))
				plug.Close(ctx)
				So(p.Wait(ctx), ShouldNotBeNil)
				So(ctx.Err(), ShouldBeNil)
			})
		})

		Convey("Plugin is not found", func() {
			plug := NewPlugin(ctx, host, []string{"doesnt_exist"})
			defer plug.Close(ctx)

			p := plug.CheckAdmission("https://good", testPin("a/b"))
			So(p.Wait(ctx), ShouldNotBeNil)
			So(ctx.Err(), ShouldBeNil)
		})

		Convey("Plugin is using unexpected protocol version", func() {
			plug := NewPlugin(ctx, host, []string{os.Args[0], "PLUGIN_NORMAL_REPLY"})
			plug.protocolVersion = 666
			defer plug.Close(ctx)

			p := plug.CheckAdmission("https://good", testPin("a/b"))
			So(p.Wait(ctx), ShouldNotBeNil)
			So(ctx.Err(), ShouldBeNil)
		})

		Convey("Plugin is crashing when connecting", func() {
			plug := NewPlugin(ctx, host, []string{os.Args[0], "PLUGIN_CRASHING_WHEN_CONNECTING"})
			defer plug.Close(ctx)

			p := plug.CheckAdmission("https://url", testPin("a/b"))
			So(p.Wait(ctx), ShouldNotBeNil)
			So(ctx.Err(), ShouldBeNil)
		})

		Convey("Plugin is crashing midway", func() {
			plug := NewPlugin(ctx, host, []string{os.Args[0], "PLUGIN_CRASH_ON_SECOND_REQUEST"})
			defer plug.Close(ctx)

			p1 := plug.CheckAdmission("https://url", testPin("a/b/1"))
			So(p1.Wait(ctx), ShouldBeNil)

			p2 := plug.CheckAdmission("https://url", testPin("a/b/2"))
			So(p2.Wait(ctx), ShouldNotBeNil)
		})

		Convey("Terminating with pending queue", func() {
			plug := NewPlugin(ctx, host, []string{os.Args[0], "PLUGIN_BLOCK_ALL_BUT_FIRST"})
			defer plug.Close(ctx)

			p1 := plug.CheckAdmission("https://url", testPin("a/b/1"))
			So(p1.Wait(ctx), ShouldBeNil)

			p2 := plug.CheckAdmission("https://url", testPin("a/b/2"))
			p3 := plug.CheckAdmission("https://url", testPin("a/b/3"))

			plug.Close(ctx)

			So(p2.Wait(ctx), ShouldNotBeNil)
			So(p3.Wait(ctx), ShouldNotBeNil)
			So(ctx.Err(), ShouldBeNil)
		})
	})
}
