// Copyright 2016 The LUCI Authors.
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

package rawpresentation

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"io"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/coordinator"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/logdog/common/viewer"
	"go.chromium.org/luci/luciexe"
	annopb "go.chromium.org/luci/luciexe/legacy/annotee/proto"

	"go.chromium.org/luci/milo/frontend/handlers/ui"
)

const (
	// DefaultLogDogHost is the default LogDog host, if one isn't specified via
	// query string.
	DefaultLogDogHost = chromeinfra.LogDogHost
)

// AnnotationStream represents a LogDog annotation protobuf stream.
type AnnotationStream struct {
	Project string
	Path    types.StreamPath

	// Client is the HTTP client to use for LogDog communication.
	Client *coordinator.Client

	// The cached Step object
	step *annopb.Step

	// Build is the build.proto, if this annotation stream is Build messages
	// instead of Step messages.
	build *bbpb.Build

	finished bool
}

// normalize validates and normalizes the stream's parameters.
func (as *AnnotationStream) normalize() error {
	if err := config.ValidateProjectName(as.Project); err != nil {
		return errors.Annotate(err, "Invalid project name: %s", as.Project).Tag(grpcutil.InvalidArgumentTag).Err()
	}

	if err := as.Path.Validate(); err != nil {
		return errors.Annotate(err, "Invalid log stream path %q", as.Path).Tag(grpcutil.InvalidArgumentTag).Err()
	}

	return nil
}

var errNotMilo = errors.New("Requested stream is not a recognized protobuf")
var errNotDatagram = errors.New("Requested stream is not a datagram stream")
var errNoEntries = errors.New("Log stream has no annotation entries")

// populateCache loads the annotation stream from LogDog and caches it on this
// AnnotationStream.
//
// If the stream does not exist, or is invalid, populateCache will return a Milo error.
func (as *AnnotationStream) populateCache(c context.Context) error {
	// Cached?
	if as.step != nil || as.build != nil {
		return nil
	}

	// Load from LogDog directly.
	log.Fields{
		"host":    as.Client.Host,
		"project": as.Project,
		"path":    as.Path,
	}.Infof(c, "Making tail request to LogDog to populateCache annotation stream.")

	var (
		state  coordinator.LogStream
		stream = as.Client.Stream(as.Project, as.Path)
	)

	le, err := stream.Tail(c, coordinator.WithState(&state), coordinator.Complete())
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to load stream.")
		return err
	}

	// Make sure that this is an annotation stream.
	switch {
	case state.Desc.StreamType != logpb.StreamType_DATAGRAM:
		return errNotDatagram

	case le == nil:
		// No annotation stream data, so render a minimal page.
		return errNoEntries
	}

	var toUnmarshal proto.Message
	var compressed bool
	var followup func()

	switch state.Desc.ContentType {
	case annopb.ContentTypeAnnotations:
		var step annopb.Step
		toUnmarshal = &step
		followup = func() {
			var latestEndedTime time.Time
			for _, sub := range step.Substep {
				switch t := sub.Substep.(type) {
				case *annopb.Step_Substep_AnnotationStream:
					// TODO(hinoka,dnj): Implement recursive / embedded substream fetching if
					// specified.
					log.Warningf(c, "Annotation stream links LogDog substream [%+v], not supported!", t.AnnotationStream)

				case *annopb.Step_Substep_Step:
					endedTime := t.Step.Ended.AsTime()
					if t.Step.Ended != nil && endedTime.After(latestEndedTime) {
						latestEndedTime = endedTime
					}
				}
			}
			if latestEndedTime.IsZero() {
				// No substep had an ended time :(
				latestEndedTime = step.Started.AsTime()
			}
			as.step = &step
		}

	case luciexe.BuildProtoZlibContentType:
		var build bbpb.Build
		toUnmarshal = &build
		compressed = true
		followup = func() {
			as.build = &build
		}

	default:
		return errNotMilo
	}

	// Get the last log entry in the stream. In reality, this will be index 0,
	// since the "Tail" call should only return one log entry.
	//
	// Because we supplied the "Complete" flag to Tail and succeeded, this
	// datagram will be complete even if its source datagram(s) are fragments.
	dg := le.GetDatagram()
	if dg == nil {
		return errors.New("Datagram stream does not have datagram data")
	}

	data := dg.Data
	if compressed {
		z, err := zlib.NewReader(bytes.NewBuffer(dg.Data))
		if err != nil {
			return errors.Annotate(
				err, "Datagram is marked as compressed, but failed to open zlib stream",
			).Err()
		}

		if data, err = io.ReadAll(z); err != nil {
			return errors.Annotate(
				err, "Datagram is marked as compressed, but failed to decompress",
			).Err()
		}
	}

	// Attempt to decode the protobuf.
	if err := proto.Unmarshal(data, toUnmarshal); err != nil {
		return err
	}
	followup()

	as.finished = (state.State.TerminalIndex >= 0 &&
		le.StreamIndex == uint64(state.State.TerminalIndex))
	return nil
}

func (as *AnnotationStream) toMiloBuild(c context.Context) *ui.MiloBuildLegacy {
	prefix, name := as.Path.Split()

	// Prepare a Streams object with only one stream.
	streams := Streams{
		MainStream: &Stream{
			Server:     as.Client.Host,
			Prefix:     string(prefix),
			Path:       string(name),
			IsDatagram: true,
			Data:       as.step,
			Closed:     as.finished,
		},
	}

	var (
		build ui.MiloBuildLegacy
		ub    = ViewerURLBuilder{
			Host:    as.Client.Host,
			Project: as.Project,
			Prefix:  prefix,
		}
	)
	AddLogDogToBuild(c, &ub, streams.MainStream.Data, &build)
	return &build
}

// ViewerURLBuilder is a URL builder that constructs LogDog viewer URLs.
type ViewerURLBuilder struct {
	Host    string
	Prefix  types.StreamName
	Project string
}

// NewURLBuilder creates a new URLBuilder that can generate links to LogDog
// pages given a LogDog StreamAddr.
func NewURLBuilder(addr *types.StreamAddr) *ViewerURLBuilder {
	prefix, _ := addr.Path.Split()
	return &ViewerURLBuilder{
		Host:    addr.Host,
		Prefix:  prefix,
		Project: addr.Project,
	}
}

// BuildLink implements URLBuilder.
func (b *ViewerURLBuilder) BuildLink(l *annopb.AnnotationLink) *ui.Link {
	switch t := l.Value.(type) {
	case *annopb.AnnotationLink_LogdogStream:
		ls := t.LogdogStream

		server := ls.Server
		if server == "" {
			server = b.Host
		}

		prefix := types.StreamName(ls.Prefix)
		if prefix == "" {
			prefix = b.Prefix
		}

		u := viewer.GetURL(server, b.Project, prefix.Join(types.StreamName(ls.Name)))
		link := ui.NewLink(l.Label, u, fmt.Sprintf("logdog link for %s", l.Label))
		if link.Label == "" {
			link.Label = ls.Name
		}
		return link

	case *annopb.AnnotationLink_Url:
		link := ui.NewLink(l.Label, t.Url, fmt.Sprintf("step link for %s", l.Label))
		if link.Label == "" {
			link.Label = "unnamed"
		}
		return link

	default:
		// Don't know how to render.
		return nil
	}
}

// GetBuild returns either a MiloBuildLegacy or a Build from a raw datagram
// stream.
//
// The type of return value is determined by the content type of the stream.
func GetBuild(c context.Context, host string, project string, path types.StreamPath) (*ui.MiloBuildLegacy, *ui.BuildPage, error) {
	as := AnnotationStream{
		Project: project,
		Path:    path,
	}
	if err := as.normalize(); err != nil {
		return nil, nil, err
	}

	// Setup our LogDog client.
	var err error
	if as.Client, err = NewClient(c, host); err != nil {
		return nil, nil, errors.Annotate(err, "generating LogDog Client").Err()
	}

	// Load the Milo annotation protobuf from the annotation stream.
	switch err := as.populateCache(c); errors.Unwrap(err) {
	case nil, errNoEntries:

	case coordinator.ErrNoSuchStream:
		return nil, nil, grpcutil.NotFoundTag.Apply(err)

	case coordinator.ErrNoAccess:
		return nil, nil, grpcutil.PermissionDeniedTag.Apply(err)

	case errNotMilo, errNotDatagram:
		// The user requested a LogDog url that isn't a Milo annotation.
		return nil, nil, grpcutil.InvalidArgumentTag.Apply(err)

	default:
		return nil, nil, errors.Annotate(err, "failed to load stream").Err()
	}

	if as.step != nil {
		return as.toMiloBuild(c), nil, nil
	}
	now := timestamppb.New(clock.Now(c))
	return nil, &ui.BuildPage{Build: ui.Build{Build: as.build, Now: now}}, nil
}

// ReadAnnotations synchronously reads and decodes the latest Step information
// from the provided StreamAddr.
func ReadAnnotations(c context.Context, addr *types.StreamAddr) (*annopb.Step, error) {
	log.Infof(c, "Loading build from LogDog stream at: %s", addr)

	client, err := NewClient(c, addr.Host)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create LogDog client").Err()
	}

	as := AnnotationStream{
		Client:  client,
		Project: addr.Project,
		Path:    addr.Path,
	}
	if err := as.normalize(); err != nil {
		return nil, errors.Annotate(err, "failed to normalize annotation stream parameters").Err()
	}

	if err := as.populateCache(c); err != nil {
		return nil, err
	}
	if as.step == nil {
		return nil, errors.New("stream does not contain annopb.Step")
	}
	return as.step, nil
}
