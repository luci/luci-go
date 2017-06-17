// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package logdog

import (
	"errors"
	"fmt"
	"time"

	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/google"
	miloProto "github.com/luci/luci-go/common/proto/milo"
	"github.com/luci/luci-go/logdog/api/logpb"
	"github.com/luci/luci-go/logdog/client/coordinator"
	"github.com/luci/luci-go/logdog/common/types"
	"github.com/luci/luci-go/logdog/common/viewer"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"
	"github.com/luci/luci-go/milo/api/resp"
	"github.com/luci/luci-go/milo/appengine/logdog/internal"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
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
		return fmt.Errorf("Invalid project name: %s", as.Project)
	}

	if err := as.Path.Validate(); err != nil {
		return fmt.Errorf("Invalid log stream path %q: %s", as.Path, err)
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
