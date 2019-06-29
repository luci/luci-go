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

package luciexe

import (
	"net/url"
	"strings"
	"sync"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler/buffered_callback"
	"go.chromium.org/luci/logdog/client/butler/bundler"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// BuildStreamName is the name of the build stream, relative to $LOGDOG_STREAM_PREFIX.
// For more details, see Executable message in
// https://chromium.googlesource.com/infra/luci/luci-go/+/master/buildbucket/proto/common.proto
const BuildStreamName = "build.proto"

// buildListener extracts a Build message from LogDog streams.
type buildListener struct {
	streamNamePrefix string // ends with slash, typically "u/".
	buildStreamName  string // build stream name, typically "u/build.proto".

	onErr func(error)

	// Covers the following fields. BEGIN(buildMU) {{{
	buildMU sync.Mutex
	// Mapping of stream name -> latest extracted Build message for this stream.
	buildProtoState map[string]*pb.Build
	// Channel of the 'merged' builds, emitted as they are computed. Will be
	// updated when the root build stream changes or when any sub-stream changes.
	mergedBuilds chan<- *pb.Build
	// }}} END(buildMU)
}

// newBuildListener creates a build listener.
//
// All recieved builds are emitted on the given builds channel.
// Root build proto will be expected at "<streamNamePrefix>/build.proto".
// All logs of its steps and of steps of sub-builds must have this prefix.
func newBuildListener(streamNamePrefix string, builds chan<- *pb.Build, onErr func(error)) *buildListener {
	if onErr == nil {
		panic("onErr is nil")
	}
	if builds == nil {
		panic("builds is nil")
	}

	streamNamePrefix = strings.TrimSuffix(streamNamePrefix, "/") + "/"
	return &buildListener{
		streamNamePrefix: streamNamePrefix,
		buildStreamName:  streamNamePrefix + BuildStreamName,
		buildProtoState:  map[string]*pb.Build{},
		mergedBuilds:     builds,
		onErr:            onErr,
	}
}

// StreamRegistrationCallback can be used as logdogServer.StreamRegistrationCallback.
func (l *buildListener) StreamRegistrationCallback(desc *logpb.LogStreamDescriptor) bundler.StreamChunkCallback {
	// TODO(iannucci): add support for capturing sub-build streams. For now only
	// capture the user application's root stream.
	if desc.Name == l.buildStreamName {
		switch {
		case desc.ContentType != protoutil.BuildMediaType:
			l.report(errors.Reason("stream %q has content type %q, expected %q", desc.Name, desc.ContentType, protoutil.BuildMediaType).Err())
		case desc.StreamType != logpb.StreamType_DATAGRAM:
			l.report(errors.Reason("stream %q has type %q, expected %q", desc.Name, desc.StreamType, logpb.StreamType_DATAGRAM).Err())
		default:
			streamName := desc.Name
			return buffered_callback.GetWrappedDatagramCallback(func(log *logpb.LogEntry) {
				if err := l.processBuildInStream(streamName, log); err != nil {
					l.report(errors.Annotate(err, "received an build.proto log entry").Err())
				}
			})
		}
	}

	return nil
}

func (l *buildListener) processBuildInStream(streamName string, log *logpb.LogEntry) error {
	dg := log.GetDatagram()
	switch {
	case dg == nil:
		return errors.Reason("no datagram").Err()
	case dg.Partial != nil:
		return errors.Reason("partial datagram, expected full").Err()
	}

	build := &pb.Build{}
	if err := proto.Unmarshal(dg.Data, build); err != nil {
		return errors.Annotate(err, "failed to unmarshal Build message").Err()
	}

	if err := l.validateBuild(build); err != nil {
		return errors.Annotate(err, "invalid Build message").Err()
	}

	// TODO(iannucci): Relay build.proto to 'build.proto' logdog stream (for
	// debugging, history, led, etc., etc.)

	l.buildMU.Lock()
	l.buildProtoState[streamName] = build
	// TODO(iannucci): Actually implement merging streams.
	if streamName == l.buildStreamName {
		l.mergedBuilds <- build
	}
	l.buildMU.Unlock()
	return nil
}

// CurrentMergedBuild returns a copy of the most recently merged Build message.
func (l *buildListener) CurrentMergedBuild() *pb.Build {
	// TODO(iannucci): return the latest merged Build here instead of the last
	// Build sent by the user.
	l.buildMU.Lock()
	current := l.buildProtoState[l.buildStreamName]
	l.buildMU.Unlock()

	if current == nil {
		return nil
	}
	return proto.Clone(current).(*pb.Build)
}

func (l *buildListener) validateBuild(build *pb.Build) error {
	for _, step := range build.Steps {
		for _, log := range step.Logs {
			u, err := url.Parse(log.Url)
			switch {
			case err != nil:
			case u.IsAbs(), u.Host != "":
				err = errors.Reason("absolute; expected relative to $LOGDOG_STREAM_PREFIX").Err()
			case !strings.HasPrefix(u.Path, l.streamNamePrefix):
				err = errors.Reason("does not start with %q", l.streamNamePrefix).Err()
			}
			if err != nil {
				return errors.Annotate(err, "invalid log URL %q in step log %q/%q", log.Url, step.Name, log.Name).Err()
			}
		}
	}

	return nil
}

// report reports a LUCI executable protocol violation via l.onErr.
func (l *buildListener) report(err error) {
	l.onErr(errors.Annotate(err, "LUCI executable protocol violation").Err())
}
