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
	"path"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler/bundler"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// buildListener listens to logdog streams and retrieves Build message.
type buildListener struct {
	StreamNamePrefix string
	OnErr            func(error)
	Build            *pb.Build
}

func (l *buildListener) StreamRegistrationCallback() func(desc *logpb.LogStreamDescriptor) bundler.StreamChunkCallback {
	buildStreamName := path.Join(l.StreamNamePrefix, "build.proto")
	return func(desc *logpb.LogStreamDescriptor) bundler.StreamChunkCallback {
		if desc.Name == buildStreamName {
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

// report reports a LUCI executable protocol violation.
func (l *buildListener) report(err error) {
	if l.OnErr != nil {
		l.OnErr(errors.Annotate(err, "LUCI executable protocol violation").Err())
	}
}
