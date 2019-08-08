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

// Package exe implements LUCI Executable protocol, documented in
// message Executable in
// https://chromium.googlesource.com/infra/luci/luci-go/+/master/buildbucket/proto/common.proto
package exe

import (
	"io/ioutil"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/luciexe"

	pb "go.chromium.org/luci/buildbucket/proto"
)

var (
	initialized int32

	testingInitialTime atomic.Value

	initializer sync.Once

	initializationErr error
	inputBuild        *pb.Build
	logdogBootstrap   *bootstrap.Bootstrap
	buildStream       streamclient.Stream
)

// ErrNotInitialized is returned from the methods in this package if you use
// them before invoking Initialize.
var ErrNotInitialized = errors.New("luciexe protocol is not initialized")

// TestingSetInitialTime allows luciexe's tests to set the initial time for the
// build.proto stream.
//
// Has no effect if Initialize has already been called.
func TestingSetInitialTime(now time.Time) {
	testingInitialTime.Store(now)
}

// Initialize initializes the current process as a LUCI Executable and allows
// the other methods in this package to work.
//
// As part of the luciexe protocol, this consumes all of the data on stdin.
//
// NOTE: Your program SHOULD call `Close()` before exiting, or there's the
// possibility of data loss.
func Initialize() error {
	initializer.Do(func() {
		defer os.Stdin.Close()
		atomic.StoreInt32(&initialized, 1)

		stdin, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			initializationErr = errors.Annotate(err, "reading stdin").Err()
			return
		}

		inputBuild = &pb.Build{}
		if err := proto.Unmarshal(stdin, inputBuild); err != nil {
			initializationErr = errors.Annotate(err, "parsing buildbucket.v2.Build from stdin").Err()
			return
		}
		if logdogBootstrap, err = bootstrap.Get(); err != nil {
			initializationErr = errors.Annotate(err, "bootstrapping Logdog client").Err()
			return
		}

		var now time.Time
		if nowI := testingInitialTime.Load(); nowI != nil {
			now = nowI.(time.Time)
		} else {
			now = time.Now()
		}

		buildStream, err = logdogBootstrap.Client.NewStream(streamproto.Flags{
			Name:        streamproto.StreamNameFlag(luciexe.BuildStreamName),
			Type:        streamproto.StreamType(logpb.StreamType_DATAGRAM),
			ContentType: protoutil.BuildMediaType,
			Timestamp:   clockflag.Time(now),
		})
		initializationErr = errors.Annotate(err, "opening build.proto stream").Err()
		return
	})
	return initializationErr
}

func assertInitialized() {
	if atomic.LoadInt32(&initialized) == 0 {
		panic(ErrNotInitialized)
	}
	if initializationErr != nil {
		panic(initializationErr)
	}
}

// GetInputBuild returns a copy of the original Build message as it was passed
// to this executable.
//
// Panics if Initialize was not called successfully.
func GetInputBuild() *pb.Build {
	assertInitialized()
	return proto.Clone(inputBuild).(*pb.Build)
}

var writeBuildLock sync.Mutex

// WriteBuild sends a new version of Build message.
//
// This method is synchronized, so it may only be called once at a time.
//
// Panics if Initialize was not called successfully.
func WriteBuild(build *pb.Build) error {
	assertInitialized()

	buf, err := proto.Marshal(build)
	if err != nil {
		return err
	}
	writeBuildLock.Lock()
	defer writeBuildLock.Unlock()
	return buildStream.WriteDatagram(buf)
}

// Close flushes all builds written with WriteBuild and closes down the client
// connection.
//
// Failing to call this before program termination may result in lost
// WriteBuilds.
//
// Panics if Initialize was not called successfully.
func Close() error {
	assertInitialized()

	writeBuildLock.Lock()
	defer writeBuildLock.Unlock()
	return buildStream.Close()
}
