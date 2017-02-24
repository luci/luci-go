// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package logdog

import (
	"fmt"
	"net/http"
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
	"github.com/luci/luci-go/milo/common/miloerror"

	"github.com/golang/protobuf/proto"
	mc "github.com/luci/gae/service/memcache"
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

	// Load from memcache, if possible. If an error occurs, we will proceed as if
	// no CachedStep was available.
	mcKey := as.memcacheKey()
	mcItem, err := mc.GetKey(c, mcKey)
	switch err {
	case nil:
		if err := proto.Unmarshal(mcItem.Value(), &as.cs); err == nil {
			return as.cs.Step, nil
		}

		// We could not unmarshal the cached value. Try and delete it from
		// memcache, since it's invalid.
		log.Fields{
			log.ErrorKey:  err,
			"memcacheKey": mcKey,
		}.Warningf(c, "Failed to unmarshal cached annotation protobuf.")
		if err := mc.Delete(c, mcKey); err != nil {
			log.WithError(err).Warningf(c, "Failed to delete invalid annotation protobuf memcache entry.")
		}

	case mc.ErrCacheMiss:
		break

	default:
		log.Fields{
			log.ErrorKey:  err,
			"memcacheKey": mcKey,
		}.Errorf(c, "Failed to load annotation protobuf memcache cached step.")
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
		return nil, &miloerror.Error{
			Message: "Requested stream is not a Milo annotation protobuf",
			Code:    http.StatusBadRequest,
		}

	case state.Desc.StreamType != logpb.StreamType_DATAGRAM:
		return nil, &miloerror.Error{
			Message: "Requested stream is not a datagram stream",
			Code:    http.StatusBadRequest,
		}

	case le == nil:
		// No annotation stream data, so render a minimal page.
		return nil, &miloerror.Error{
			Message: "Log stream has no annotation entries",
			Code:    http.StatusNotFound,
		}
	}

	// Get the last log entry in the stream. In reality, this will be index 0,
	// since the "Tail" call should only return one log entry.
	//
	// Because we supplied the "Complete" flag to Tail and suceeded, this datagram
	// will be complete even if its source datagram(s) are fragments.
	dg := le.GetDatagram()
	if dg == nil {
		return nil, &miloerror.Error{
			Message: "Datagram stream does not have datagram data",
			Code:    http.StatusInternalServerError,
		}
	}

	// Attempt to decode the Step protobuf.
	var step miloProto.Step
	if err := proto.Unmarshal(dg.Data, &step); err != nil {
		return nil, &miloerror.Error{
			Message: "Failed to unmarshal annotation protobuf",
			Code:    http.StatusInternalServerError,
		}
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

	// Marshal and cache the step. If this is the final protobuf in the stream,
	// cache it indefinitely; otherwise, cache it for intermediateCacheLifetime.
	//
	// If this fails, it is non-fatal.
	mcData, err := proto.Marshal(&as.cs)
	if err == nil {
		mcItem = mc.NewItem(c, mcKey)
		if !as.cs.Finished {
			mcItem.SetExpiration(intermediateCacheLifetime)
		}
		mcItem.SetValue(mcData)
		if err := mc.Set(c, mcItem); err != nil {
			log.WithError(err).Warningf(c, "Failed to cache annotation protobuf CachedStep.")
		}
	} else {
		log.WithError(err).Warningf(c, "Failed to marshal annotation protobuf CachedStep.")
	}

	return as.cs.Step, nil
}

func (as *AnnotationStream) memcacheKey() string {
	return fmt.Sprintf("logdog/%s/%s/%s", as.Client.Host, as.Project, as.Path)
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
		link := resp.Link{
			Label: l.Label,
			URL:   u,
		}
		if link.Label == "" {
			link.Label = ls.Name
		}
		return &link

	case *miloProto.Link_Url:
		link := resp.Link{
			Label: l.Label,
			URL:   t.Url,
		}
		if link.Label == "" {
			link.Label = "unnamed"
		}
		return &link

	default:
		// Don't know how to render.
		return nil
	}
}
