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

package buildspy

import (
	"net/url"
	"strings"
	"sync"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/buildbucket/luciexe"
	"go.chromium.org/luci/buildbucket/luciexe/runner/runnerbutler"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler/bundler"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// Data represents a (build, err) tuple.
//
// Every time a Spy intercepts a *Build, it will emit this. If the Spy
// encounters an error, it will emit this and close the channel.
type Data struct {
	Build *pb.Build
	Err   error
}

// Spy extracts a Build message from LogDog streams.
type Spy struct {
	streamNamePrefix string // ends with slash, typically "u/".
	buildStreamName  string // build stream name, typically "u/build.proto".

	buildMU sync.Mutex // protects assignment to sendC

	// sendC will be nil when this Spy is stopped.
	sendC chan<- Data
	C     <-chan Data
}

// New creates a build spy.
//
// Root build proto will be expected at "<streamNamePrefix>/build.proto".
// All logs of its steps and of steps of sub-builds must have this prefix.
//
// The user of this Spy must drain `C` at all times; failure to do so will
// potentially block all incoming work to the logdog butler this Spy is attached
// to.
func New(streamNamePrefix string) *Spy {
	streamNamePrefix = strings.TrimSuffix(streamNamePrefix, "/") + "/"
	ch := make(chan Data, 16)
	return &Spy{
		streamNamePrefix: streamNamePrefix,
		buildStreamName:  streamNamePrefix + luciexe.BuildStreamName,
		sendC:            ch,
		C:                ch,
	}
}

// On must be used to associate this Spy with a butler service. Otherwise the
// Spy does nothing.
func (l *Spy) On(s *runnerbutler.Server) {
	prev := s.StreamRegistrationCallback
	s.StreamRegistrationCallback = func(desc *logpb.LogStreamDescriptor) bundler.StreamChunkCallback {
		if !l.checkStopped() && desc.Name == l.buildStreamName {
			switch {
			case desc.ContentType != protoutil.BuildMediaType:
				l.stop(errors.Reason("stream %q has content type %q, expected %q", desc.Name, desc.ContentType, protoutil.BuildMediaType).Err())
			case desc.StreamType != logpb.StreamType_DATAGRAM:
				l.stop(errors.Reason("stream %q has type %q, expected %q", desc.Name, desc.StreamType, logpb.StreamType_DATAGRAM).Err())
			default:
				return l.onBuildChunk
			}
		}
		// TODO(iannucci): add support for sub-builds.
		if prev != nil {
			return prev(desc)
		}
		return nil
	}
}

func (l *Spy) onBuildChunk(log *logpb.LogEntry) {
	if l.checkStopped() {
		return
	}

	if log == nil { // flush
		l.stop(nil)
		return
	}

	if err := l.processBuildLogEntry(log); err != nil {
		l.stop(errors.Annotate(err, "received build.proto log entry").Err())
	}
}

func (l *Spy) processBuildLogEntry(log *logpb.LogEntry) error {
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

	l.buildMU.Lock()
	defer l.buildMU.Unlock()
	if l.sendC == nil {
		return nil
	}
	l.sendC <- Data{build, nil}

	return nil
}

func (l *Spy) validateBuild(build *pb.Build) error {
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

// stop causes the Spy to stop all monitoring operations.
//
// If `err` is not nil, this reports a LUCI executable protocol violation via
// s.C before stopping.
//
// This closes s.C to unblock downstream clients and makes sendC nil.
func (l *Spy) stop(err error) {
	l.buildMU.Lock()
	defer l.buildMU.Unlock()

	if l.sendC == nil {
		return
	}
	if err != nil {
		l.sendC <- Data{
			Err: errors.Annotate(err, "LUCI executable protocol violation").Err(),
		}
	}
	close(l.sendC)
	l.sendC = nil
}

func (l *Spy) checkStopped() bool {
	l.buildMU.Lock()
	defer l.buildMU.Unlock()
	return l.sendC == nil
}
