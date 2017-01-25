// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package logdog

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/google"
	miloProto "github.com/luci/luci-go/common/proto/milo"
	"github.com/luci/luci-go/grpc/grpcutil"
	"github.com/luci/luci-go/logdog/api/logpb"
	"github.com/luci/luci-go/logdog/client/coordinator"
	"github.com/luci/luci-go/logdog/common/types"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"
	"github.com/luci/luci-go/milo/api/resp"
	"github.com/luci/luci-go/milo/appengine/logdog/internal"
	"github.com/luci/luci-go/milo/common/miloerror"

	"github.com/golang/protobuf/proto"
	mc "github.com/luci/gae/service/memcache"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
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

type annotationStreamRequest struct {
	*AnnotationStream

	// host is the name of the LogDog host.
	host string

	project cfgtypes.ProjectName
	path    types.StreamPath

	// logDogClient is the HTTP client to use for LogDog communication.
	logDogClient *coordinator.Client

	// cs is the unmarshalled annotation stream Step and associated data.
	cs internal.CachedStep
}

func (as *annotationStreamRequest) normalize() error {
	if err := as.project.Validate(); err != nil {
		return &miloerror.Error{
			Message: "Invalid project name",
			Code:    http.StatusBadRequest,
		}
	}

	if err := as.path.Validate(); err != nil {
		return &miloerror.Error{
			Message: fmt.Sprintf("Invalid log stream path %q: %s", as.path, err),
			Code:    http.StatusBadRequest,
		}
	}

	// Get the host. We normalize it to lowercase and trim spaces since we use
	// it as a memcache key.
	as.host = strings.ToLower(strings.TrimSpace(as.host))
	if as.host == "" {
		as.host = defaultLogDogHost
	}
	if strings.ContainsRune(as.host, '/') {
		return errors.New("invalid host name")
	}

	return nil
}

func (as *annotationStreamRequest) memcacheKey() string {
	return fmt.Sprintf("logdog/%s/%s/%s", as.host, as.project, as.path)
}

func (as *annotationStreamRequest) load(c context.Context) error {
	// Load from memcache, if possible. If an error occurs, we will proceed as if
	// no CachedStep was available.
	mcKey := as.memcacheKey()
	mcItem, err := mc.GetKey(c, mcKey)
	switch err {
	case nil:
		if err := proto.Unmarshal(mcItem.Value(), &as.cs); err == nil {
			return nil
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
		"project": as.project,
		"path":    as.path,
		"host":    as.host,
	}.Infof(c, "Making tail request to LogDog to fetch annotation stream.")

	var (
		state  coordinator.LogStream
		stream = as.logDogClient.Stream(as.project, as.path)
	)
	le, err := stream.Tail(c, coordinator.WithState(&state), coordinator.Complete())
	switch code := grpcutil.Code(err); code {
	case codes.OK:
		break

	case codes.NotFound:
		return &miloerror.Error{
			Message: "Stream not found",
			Code:    http.StatusNotFound,
		}

	default:
		// TODO: Once we switch to delegation tokens and are making the request on
		// behalf of a user rather than the Milo service, handle PermissionDenied.
		log.Fields{
			log.ErrorKey: err,
			"code":       code,
		}.Errorf(c, "Failed to load LogDog annotation stream.")
		return &miloerror.Error{
			Message: "Failed to load stream",
			Code:    http.StatusInternalServerError,
		}
	}

	// Make sure that this is an annotation stream.
	switch {
	case state.Desc.ContentType != miloProto.ContentTypeAnnotations:
		return &miloerror.Error{
			Message: "Requested stream is not a Milo annotation protobuf",
			Code:    http.StatusBadRequest,
		}

	case state.Desc.StreamType != logpb.StreamType_DATAGRAM:
		return &miloerror.Error{
			Message: "Requested stream is not a datagram stream",
			Code:    http.StatusBadRequest,
		}

	case le == nil:
		// No annotation stream data, so render a minimal page.
		return nil
	}

	// Get the last log entry in the stream. In reality, this will be index 0,
	// since the "Tail" call should only return one log entry.
	//
	// Because we supplied the "Complete" flag to Tail and suceeded, this datagram
	// will be complete even if its source datagram(s) are fragments.
	dg := le.GetDatagram()
	if dg == nil {
		return &miloerror.Error{
			Message: "Datagram stream does not have datagram data",
			Code:    http.StatusInternalServerError,
		}
	}

	// Attempt to decode the Step protobuf.
	var step miloProto.Step
	if err := proto.Unmarshal(dg.Data, &step); err != nil {
		return &miloerror.Error{
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

	// Annotee is apparently not putting an ended time on some annotation protos.
	// This hack will ensure that a finished build will always have an ended time.
	if as.cs.Finished && as.cs.Step.Ended == nil {
		as.cs.Step.Ended = google.NewTimestamp(latestEndedTime)
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

	return nil
}

func (as *annotationStreamRequest) toMiloBuild(c context.Context) *resp.MiloBuild {
	prefix, name := as.path.Split()

	// Prepare a Streams object with only one stream.
	streams := Streams{
		MainStream: &Stream{
			Server:     as.host,
			Prefix:     string(prefix),
			Path:       string(name),
			IsDatagram: true,
			Data:       as.cs.Step,
			Closed:     as.cs.Finished,
		},
	}

	var (
		build resp.MiloBuild
		ub    = logDogURLBuilder{
			project: as.project,
			host:    as.host,
			prefix:  prefix,
		}
	)
	AddLogDogToBuild(c, &ub, streams.MainStream.Data, &build)
	return &build
}

type logDogURLBuilder struct {
	host    string
	prefix  types.StreamName
	project cfgtypes.ProjectName
}

func (b *logDogURLBuilder) BuildLink(l *miloProto.Link) *resp.Link {
	switch t := l.Value.(type) {
	case *miloProto.Link_LogdogStream:
		ls := t.LogdogStream

		server := ls.Server
		if server == "" {
			server = b.host
		}

		prefix := types.StreamName(ls.Prefix)
		if prefix == "" {
			prefix = b.prefix
		}

		path := fmt.Sprintf("%s/%s", b.project, prefix.Join(types.StreamName(ls.Name)))
		u := url.URL{
			Scheme: "https",
			Host:   server,
			Path:   "v/",
			RawQuery: url.Values{
				"s": []string{string(path)},
			}.Encode(),
		}

		link := resp.Link{
			Label: l.Label,
			URL:   u.String(),
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
