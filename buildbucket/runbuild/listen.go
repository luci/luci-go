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

package runbuild

import (
	"net/url"
	"strings"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler/bundler"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// buildListener extracts a Build message from LogDog streams.
type buildListener struct {
	Build *pb.Build

	streamNamePrefix    string
	onErr               func(error)
	buildStreamName     string
	namePrefixWithSlash string
}

// newBuildListener creates a build listener.
//
// Root build proto will be expected at "<streamNamePrefix>/build.proto".
// All logs of its steps and steps of sub-builds must start have this prefix.
func newBuildListener(streamNamePrefix string, onErr func(error)) *buildListener {
	streamNamePrefix = strings.TrimSuffix(streamNamePrefix, "/") + "/"
	return &buildListener{
		streamNamePrefix: streamNamePrefix,
		buildStreamName:  streamNamePrefix + "build.proto",
		onErr:            onErr,
	}
}

// StreamRegistrationCallback can be used as logdogServer.StreamRegistrationCallback.
func (l *buildListener) StreamRegistrationCallback(desc *logpb.LogStreamDescriptor) bundler.StreamChunkCallback {
	if desc.Name == l.buildStreamName {
		switch {
		case desc.ContentType != protoutil.BuildMediaType:
			l.report(errors.Reason("stream %q has content type %q, expected %q", desc.Name, desc.ContentType, protoutil.BuildMediaType).Err())
		case desc.StreamType != logpb.StreamType_DATAGRAM:
			l.report(errors.Reason("stream %q has type %q, expected %q", desc.Name, desc.StreamType, logpb.StreamType_DATAGRAM).Err())
		default:
			return l.onBuildChunk
		}
	}

	return nil
}

func (l *buildListener) onBuildChunk(log *logpb.LogEntry) {
	if err := l.processBuildLogEntry(log); err != nil {
		l.report(err)
	}
}

func (l *buildListener) processBuildLogEntry(log *logpb.LogEntry) error {
	dg := log.GetDatagram()
	switch {
	case dg == nil:
		return errors.Reason("received a build.proto log entry without a datagram").Err()
	case dg.Partial != nil:
		return errors.Reason("received a partial build.proto log entry datagram, expected full").Err()
	}

	build := &pb.Build{}
	if err := proto.Unmarshal(dg.Data, build); err != nil {
		return errors.Annotate(err, "invalid build.proto log entry").Err()
	}

	l.Build = build

	// TODO(nodir): make UpdateBuild call.

	return nil
}

func (l *buildListener) validateBuild(build *pb.Build) error {
	for _, step := range build.Steps {
		for _, log := range step.Logs {
			u, err := url.Parse(log.Url)
			switch {
			case err != nil:
			case u.IsAbs(), u.Host != "":
				err = errors.Reason("absolute; expected relative to $LOGDOG_STREAM_PREFIX").Err()
			case !strings.HasPrefix(u.Path, l.namePrefixWithSlash):
				err = errors.Reason("does not start with %q", l.namePrefixWithSlash).Err()
			}
			if err != nil {
				return errors.Annotate(err, "invalid log URL %q in step log %q/%q", log.Url, step.Name, log.Name).Err()
			}
		}
	}

	return nil
}

// report reports a LUCI executable protocol violation.
func (l *buildListener) report(err error) {
	if l.onErr != nil {
		l.onErr(errors.Annotate(err, "LUCI executable protocol violation").Err())
	}
}
