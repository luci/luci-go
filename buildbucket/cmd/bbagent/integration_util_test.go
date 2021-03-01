// Copyright 2021 The LUCI Authors.
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

package main

import (
	"context"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/rpc"
	"go.chromium.org/luci/buildbucket/cmd/bbagent/bbinput"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/gae/filter/txndefer"
	memoryds "go.chromium.org/luci/gae/impl/memory"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/prpc"
	ldMemory "go.chromium.org/luci/logdog/client/butler/output/memory"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq"
)

const fakeUpdateToken = "deadbeef1234567890"

func startFakeBuildbucketBuildService() (addr net.Addr, shutdown func(), scheduleBuild func() int64, getBuild func(int64) *bbpb.Build) {
	ctx := memoryds.Use(context.Background())
	ctx = txndefer.FilterRDS(ctx)

	if testing.Verbose() {
		ctx = gologger.StdConfig.Use(ctx)
	} else {
		ctx = logging.SetFactory(ctx, func(ctx context.Context) logging.Logger {
			return logging.Null
		})
	}
	ctx, testClock := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
	ctx = auth.WithState(ctx, &authtest.FakeState{
		Identity:       "user:fake_user@example.com",
		IdentityGroups: []string{"buildbucket-update-build-users"},
	})
	ctx, _ = tq.TestingContext(ctx, nil)

	psrv := &prpc.Server{
		Authenticator:            prpc.NoAuthentication,
		HackFixFieldMasksForJSON: true,
		UnaryServerInterceptor: func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			testClock.Add(time.Second) // every server call advances time by one second
			return handler(ctx, req)
		},
	}

	bbpb.RegisterBuildsServer(psrv, rpc.NewBuilds())
	r := router.NewWithRootContext(ctx)
	psrv.InstallHandlers(r, router.MiddlewareChain{})
	srv := &http.Server{Handler: r}

	ln, err := net.Listen("tcp", "localhost:")
	if err != nil {
		panic(err)
	}

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	addr = ln.Addr()

	shutdown = func() {
		if err := srv.Shutdown(context.Background()); err != nil {
			panic(err)
		}
		if err := <-errCh; err != nil {
			panic(err)
		}
	}

	scheduleBuild = func() int64 {
		m := &model.Build{
			Proto: bbpb.Build{
				Input: &bbpb.Build_Input{},
				Builder: &bbpb.BuilderID{
					Project: "fake_project",
					Bucket:  "fake_project/bucket",
					Builder: "fake_builder",
				},
				Status:      bbpb.Status_SCHEDULED,
				GracePeriod: durationpb.New(25 * time.Second),
			},
			UpdateToken: fakeUpdateToken,
		}
		So(ds.AllocateIDs(ctx, m), ShouldBeNil)
		m.Proto.Id = m.ID
		So(ds.Put(ctx, m), ShouldBeNil)
		return m.ID
	}

	getBuild = func(id int64) *bbpb.Build {
		m := &model.Build{ID: id}
		So(ds.Get(ctx, m), ShouldBeNil)
		ret, err := m.ToProto(ctx, mask.All(&bbpb.Build{}))
		So(err, ShouldBeNil)
		return ret
	}

	return
}

func runBbagent(addr net.Addr, base *bbpb.Build, tc bbagentTestCase) (int, *ldMemory.Output) {
	exe, err := os.Executable()
	So(err, ShouldBeNil)

	wd, err := ioutil.TempDir("", "bbagent-test-wd-*")
	So(err, ShouldBeNil)
	defer os.RemoveAll(wd)

	input := &bbpb.BBAgentArgs{
		PayloadPath: filepath.Dir(exe),
		CacheDir:    filepath.Join(wd, "cache"),
		Build:       proto.Clone(base).(*bbpb.Build),
	}
	So(os.Mkdir(input.CacheDir, 0777), ShouldBeNil)
	proto.Merge(input.Build, &bbpb.Build{
		Exe: &bbpb.Executable{
			Cmd: []string{filepath.Base(exe)},
		},
		Infra: &bbpb.BuildInfra{
			Buildbucket: &bbpb.BuildInfra_Buildbucket{
				Hostname: addr.String(),
			},
			Swarming: &bbpb.BuildInfra_Swarming{
				Hostname: "fake.swarming.example.com",
				TaskId:   "fa83234567891",
			},
			Logdog: &bbpb.BuildInfra_LogDog{
				Hostname: "test.example.com",
			},
		},
	})

	sbytes, err := proto.Marshal(&bbpb.BuildSecrets{
		BuildToken: fakeUpdateToken,
	})
	So(err, ShouldBeNil)

	ctx := lucictx.SetSwarming(context.Background(), &lucictx.Swarming{SecretBytes: sbytes})

	logData := &ldMemory.Output{}
	ctx = context.WithValue(ctx, &testingOutputCtxKey, logData)

	// TODO(iannucci): fix host and invoke to work properly with ctx-provided
	// environment tweaks.
	os.Setenv(TestTargetProgEnvKey, tc.name)
	defer os.Unsetenv(TestTargetProgEnvKey)

	// Yuck. Needed on bots to prevent the logdog namespace from leaking into the
	// test.
	oldNs := os.Getenv(bootstrap.EnvNamespace)
	os.Setenv(bootstrap.EnvNamespace, "")
	defer os.Setenv(bootstrap.EnvNamespace, oldNs)

	return mainImpl(ctx, wd, []string{"bbagent_test", bbinput.Encode(input)}), logData
}

const TestTargetProgEnvKey = "BBAGENT_TEST_PROG"

type bbagentTestCase struct {
	name   string
	prog   func()
	testFn testFnT
}

type testFnT func(runBbagent func(*bbpb.Build) (int, *ldMemory.Output), getBuild func() *bbpb.Build)

var targetPrograms = map[string]bbagentTestCase{}

func declareBBAgentTestCase(name string, prog func(), testFn testFnT) bbagentTestCase {
	if name == "" {
		panic(errors.New("declareBBAgentTestCase without name"))
	}
	if _, ok := targetPrograms[name]; ok {
		panic(errors.Reason("declareBBAgentTestCase %q registered twice", name).Err())
	}
	ret := bbagentTestCase{name, prog, testFn}
	targetPrograms[name] = ret
	return ret
}

func init() {
	env := environ.System()
	if targProg, ok := env.Get(TestTargetProgEnvKey); ok {
		if targProg == "" {
			panic(errors.Reason("env key %q set without value", TestTargetProgEnvKey).Err())
		}
		prog := targetPrograms[targProg].prog
		if prog == nil {
			panic(errors.Reason("nil target program %q", targProg).Err())
		}
		prog()
		os.Exit(66) // prog should exit by itself, but just in case it doesn't
	}
}
