// Copyright 2019 The LUCI Authors.
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

package exe

import (
	"context"
	"io/ioutil"
	"os"
	"sync"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/luciexe"

	pb "go.chromium.org/luci/buildbucket/proto"
)

type exeClient struct {
	mu sync.RWMutex

	initialized bool

	initializationErr error
	inputBuild        *pb.Build
	logdogBootstrap   *bootstrap.Bootstrap
	buildStream       streamclient.Stream
}

func (exe *exeClient) initialize(ctx context.Context) error {
	exe.mu.Lock()
	defer exe.mu.Unlock()

	if !exe.initialized {
		defer os.Stdin.Close()

		stdin, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			exe.initializationErr = errors.Annotate(err, "reading stdin").Err()
			return exe.initializationErr
		}

		exe.inputBuild = &pb.Build{}
		if err := proto.Unmarshal(stdin, exe.inputBuild); err != nil {
			exe.initializationErr = errors.Annotate(err, "parsing buildbucket.v2.Build from stdin").Err()
			return exe.initializationErr
		}
		if exe.logdogBootstrap, err = bootstrap.Get(); err != nil {
			exe.initializationErr = errors.Annotate(err, "bootstrapping Logdog client").Err()
			return exe.initializationErr
		}

		exe.buildStream, err = exe.logdogBootstrap.Client.NewStream(streamproto.Flags{
			Name:        streamproto.StreamNameFlag(luciexe.BuildStreamName),
			Type:        streamproto.StreamType(logpb.StreamType_DATAGRAM),
			ContentType: protoutil.BuildMediaType,
			Timestamp:   clockflag.Time(clock.Now(ctx)),
		})
		exe.initializationErr = errors.Annotate(err, "opening build.proto stream").Err()
		exe.initialized = true
	}
	return exe.initializationErr
}

func (exe *exeClient) assertInitialized() {
	exe.mu.RLock()
	defer exe.mu.RUnlock()
	if !exe.initialized {
		panic(ErrNotInitialized)
	}
	if exe.initializationErr != nil {
		panic(exe.initializationErr)
	}
}

func (exe *exeClient) getInputBuild() *pb.Build {
	exe.assertInitialized()
	return proto.Clone(exe.inputBuild).(*pb.Build)
}

func dumbCancelable(ctx context.Context, fn func() error) error {
	// TODO(iannucci): implement context cancellation/deadline inside
	// buildStream instead, since it's likely socket-based.

	done := make(chan error)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (exe *exeClient) writeBuild(ctx context.Context, build *pb.Build) error {
	exe.assertInitialized()

	buf, err := proto.Marshal(build)
	if err != nil {
		return err
	}

	exe.mu.Lock()
	defer exe.mu.Unlock()
	return dumbCancelable(ctx, func() error {
		return exe.buildStream.WriteDatagram(buf)
	})
}

func (exe *exeClient) closeBuildStream(ctx context.Context) error {
	exe.assertInitialized()

	exe.mu.Lock()
	defer exe.mu.Unlock()
	return dumbCancelable(ctx, exe.buildStream.Close)
}
