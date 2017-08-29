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
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	miloProto "go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/coordinator"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/logdog/common/viewer"
	"go.chromium.org/luci/luci_config/common/cfgtypes"
	"go.chromium.org/luci/milo/api/resp"
	"go.chromium.org/luci/milo/buildsource/rawpresentation/internal"
	"go.chromium.org/luci/milo/common"
)

const (
	// intermediateCacheLifetime is the amount of time to cache intermediate (non-
	// terminal) annotation streams. Terminal annotation streams are cached
	// indefinitely.
	intermediateCacheLifetime = 10 * time.Second

	// defaultLogDogHost is the default LogDog host, if one isn't specified via
	// query string.
	defaultLogDogHost = "luci-logdog.appspot.com"
)

// AnnotationStream represents a LogDog annotation protobuf stream.
type AnnotationStream struct {
	Project cfgtypes.ProjectName
	Path    types.StreamPath

	// Client is the HTTP client to use for LogDog communication.
	Client *coordinator.Client

	// cs is the unmarshalled annotation stream Step and associated data.
	cs internal.CachedStep
}

// Normalize validates and normalizes the stream's parameters.
func (as *AnnotationStream) Normalize() error {
	if err := as.Project.Validate(); err != nil {
		return errors.Annotate(err, "Invalid project name: %s", as.Project).Tag(common.CodeParameterError).Err()
	}

	if err := as.Path.Validate(); err != nil {
		return errors.Annotate(err, "Invalid log stream path %q", as.Path).Tag(common.CodeParameterError).Err()
	}

	return nil
}

var errNotMilo = errors.New("Requested stream is not a Milo annotation protobuf")
var errNotDatagram = errors.New("Requested stream is not a datagram stream")
var errNoEntries = errors.New("Log stream has no annotation entries")

// Fetch loads the annotation stream from LogDog.
//
// If the stream does not exist, or is invalid, Fetch will return a Milo error.
// Otherwise, it will return the Step that was loaded.
//
// Fetch caches the step, so multiple calls to Fetch will return the same Step
// value.
func (as *AnnotationStream) Fetch(c context.Context) (*miloProto.Step, error) {
	// Cached?
	if as.cs.Step != nil {
		return as.cs.Step, nil
	}

	// Load from LogDog directly.
	log.Fields{
		"host":    as.Client.Host,
		"project": as.Project,
		"path":    as.Path,
	}.Infof(c, "Making tail request to LogDog to fetch annotation stream.")

	var (
		state  coordinator.LogStream
		stream = as.Client.Stream(as.Project, as.Path)
	)

	le, err := stream.Tail(c, coordinator.WithState(&state), coordinator.Complete())
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to load stream.")
		return nil, err
	}

	// Make sure that this is an annotation stream.
	switch {
	case state.Desc.ContentType != miloProto.ContentTypeAnnotations:
		return nil, errNotMilo

	case state.Desc.StreamType != logpb.StreamType_DATAGRAM:
		return nil, errNotDatagram

	case le == nil:
		// No annotation stream data, so render a minimal page.
		return nil, errNoEntries
	}

	// Get the last log entry in the stream. In reality, this will be index 0,
	// since the "Tail" call should only return one log entry.
	//
	// Because we supplied the "Complete" flag to Tail and suceeded, this datagram
	// will be complete even if its source datagram(s) are fragments.
	dg := le.GetDatagram()
	if dg == nil {
		return nil, errors.New("Datagram stream does not have datagram data")
	}

	// Attempt to decode the Step protobuf.
	var step miloProto.Step
	if err := proto.Unmarshal(dg.Data, &step); err != nil {
		return nil, err
	}

	var latestEndedTime time.Time
	for _, sub := range step.Substep {
		switch t := sub.Substep.(type) {
		case *miloProto.Step_Substep_AnnotationStream:
			// TODO(hinoka,dnj): Implement recursive / embedded substream fetching if
			// specified.
			log.Warningf(c, "Annotation stream links LogDog substream [%+v], not supported!", t.AnnotationStream)

		case *miloProto.Step_Substep_Step:
			endedTime := google.TimeFromProto(t.Step.Ended)
			if t.Step.Ended != nil && endedTime.After(latestEndedTime) {
				latestEndedTime = endedTime
			}
		}
	}
	if latestEndedTime.IsZero() {
		// No substep had an ended time :(
		latestEndedTime = google.TimeFromProto(step.Started)
	}

	// Build our CachedStep.
	as.cs = internal.CachedStep{
		Step:     &step,
		Finished: (state.State.TerminalIndex >= 0 && le.StreamIndex == uint64(state.State.TerminalIndex)),
	}
	return as.cs.Step, nil
}

func (as *AnnotationStream) toMiloBuild(c context.Context) *resp.MiloBuild {
	prefix, name := as.Path.Split()

	// Prepare a Streams object with only one stream.
	streams := Streams{
		MainStream: &Stream{
			Server:     as.Client.Host,
			Prefix:     string(prefix),
			Path:       string(name),
			IsDatagram: true,
			Data:       as.cs.Step,
			Closed:     as.cs.Finished,
		},
	}

	var (
		build resp.MiloBuild
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
	Project cfgtypes.ProjectName
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
func (b *ViewerURLBuilder) BuildLink(l *miloProto.Link) *resp.Link {
	switch t := l.Value.(type) {
	case *miloProto.Link_LogdogStream:
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
		link := resp.NewLink(l.Label, u)
		if link.Label == "" {
			link.Label = ls.Name
		}
		return link

	case *miloProto.Link_Url:
		link := resp.NewLink(l.Label, t.Url)
		if link.Label == "" {
			link.Label = "unnamed"
		}
		return link

	default:
		// Don't know how to render.
		return nil
	}
}

// BuildID implements buildsource.ID.
type BuildID struct {
	Host    string
	Project cfgtypes.ProjectName
	Path    types.StreamPath
}

// GetLog implements buildsource.ID.
func (b *BuildID) GetLog(context.Context, string) (string, bool, error) { panic("not implemented") }

// Get implements buildsource.ID.
func (b *BuildID) Get(c context.Context) (*resp.MiloBuild, error) {
	as := AnnotationStream{
		Project: b.Project,
		Path:    b.Path,
	}
	if err := as.Normalize(); err != nil {
		return nil, err
	}

	// Setup our LogDog client.
	var err error
	if as.Client, err = NewClient(c, b.Host); err != nil {
		return nil, errors.Annotate(err, "generating LogDog Client").Err()
	}

	// Load the Milo annotation protobuf from the annotation stream.
	switch _, err := as.Fetch(c); errors.Unwrap(err) {
	case nil, errNoEntries:

	case coordinator.ErrNoSuchStream:
		return nil, common.CodeNotFound.Tag().Apply(err)

	case coordinator.ErrNoAccess:
		return nil, common.CodeNoAccess.Tag().Apply(err)

	case errNotMilo, errNotDatagram:
		// The user requested a LogDog url that isn't a Milo annotation.
		return nil, common.CodeParameterError.Tag().Apply(err)

	default:
		return nil, errors.Annotate(err, "failed to load stream").Err()
	}

	return as.toMiloBuild(c), nil
}

// NewBuildID generates a new un-validated BuildID.
func NewBuildID(host, project, path string) *BuildID {
	return &BuildID{
		strings.TrimSpace(host),
		cfgtypes.ProjectName(project),
		types.StreamPath(strings.Trim(path, "/")),
	}
}
