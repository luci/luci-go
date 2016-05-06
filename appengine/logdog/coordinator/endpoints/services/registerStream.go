// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"crypto/subtle"
	"errors"
	"time"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/mutations"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/grpcutil"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

func matchesLogStream(r *logdog.RegisterStreamRequest, ls *coordinator.LogStream) error {
	if r.Path != string(ls.Path()) {
		return errors.New("paths do not match")
	}

	if subtle.ConstantTimeCompare(r.Secret, ls.Secret) != 1 {
		return errors.New("secrets do not match")
	}

	if r.ProtoVersion != ls.ProtoVersion {
		return errors.New("protobuf version does not match")
	}

	dv, err := ls.DescriptorValue()
	if err != nil {
		return errors.New("log stream has invalid descriptor value")
	}
	if !dv.Equal(r.Desc) {
		return errors.New("descriptor protobufs do not match")
	}

	return nil
}

func loadLogStreamState(project config.ProjectName, ls *coordinator.LogStream) *logdog.LogStreamState {
	st := logdog.LogStreamState{
		Project:       string(project),
		Path:          string(ls.Path()),
		ProtoVersion:  ls.ProtoVersion,
		TerminalIndex: ls.TerminalIndex,
		Archived:      ls.Archived(),
		Purged:        ls.Purged,
	}
	if !ls.Terminated() {
		st.TerminalIndex = -1
	}
	return &st
}

// RegisterStream is an idempotent stream state register operation.
func (s *server) RegisterStream(c context.Context, req *logdog.RegisterStreamRequest) (*logdog.RegisterStreamResponse, error) {
	log.Fields{
		"project": req.Project,
		"path":    req.Path,
	}.Infof(c, "Registration request for log stream.")

	path := types.StreamPath(req.Path)
	if err := path.Validate(); err != nil {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Invalid path (%s): %s", path, err)
	}

	switch {
	case req.ProtoVersion == "":
		return nil, grpcutil.Errf(codes.InvalidArgument, "No protobuf version supplied.")
	case req.ProtoVersion != logpb.Version:
		return nil, grpcutil.Errf(codes.InvalidArgument, "Unrecognized protobuf version.")
	case req.Desc == nil:
		return nil, grpcutil.Errf(codes.InvalidArgument, "Missing log stream descriptor.")
	}

	secret := types.PrefixSecret(req.Secret)
	if err := secret.Validate(); err != nil {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Invalid prefix secret: %s", err)
	}

	prefix, name := path.Split()
	if err := req.Desc.Validate(true); err != nil {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Invalid log stream descriptor: %s", err)
	}
	switch {
	case req.Desc.Prefix != string(prefix):
		return nil, grpcutil.Errf(codes.InvalidArgument, "Descriptor prefix does not match path (%s != %s)",
			req.Desc.Prefix, prefix)
	case req.Desc.Name != string(name):
		return nil, grpcutil.Errf(codes.InvalidArgument, "Descriptor name does not match path (%s != %s)",
			req.Desc.Name, name)
	}

	// Load our config and archive expiration.
	_, cfg, err := coordinator.GetServices(c).Config(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to load configuration.")
		return nil, grpcutil.Internal
	}

	archiveDelayMax := cfg.Coordinator.ArchiveDelayMax.Duration()
	if archiveDelayMax < 0 {
		log.Fields{
			"archiveDelayMax": archiveDelayMax,
		}.Errorf(c, "Must have positive maximum archive delay.")
		return nil, grpcutil.Internal
	}

	// Already registered? (Non-transactional).
	ls := coordinator.LogStreamFromPath(path)
	switch err := ds.Get(c).Get(ls); err {
	case nil:
		// We want this to be idempotent, so validate that it matches the current
		// configuration and return accordingly.
		if err := matchesLogStream(req, ls); err != nil {
			return nil, grpcutil.Errf(codes.AlreadyExists, "Log stream is already incompatibly registered: %v", err)
		}

	case ds.ErrNoSuchEntity:
		// The registration is valid, so retain it.
		err = tumble.RunMutation(c, &registerStreamMutation{
			LogStream:    ls,
			req:          req,
			archiveDelay: archiveDelayMax,
		})
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
			}.Errorf(c, "Failed to register LogStream.")
			return nil, filterError(err)
		}

	default:
		log.WithError(err).Errorf(c, "Failed to check for log stream.")
		return nil, grpcutil.Internal
	}

	return &logdog.RegisterStreamResponse{
		State:  loadLogStreamState(coordinator.Project(c), ls),
		Secret: ls.Secret,
	}, nil
}

func filterError(err error) error {
	switch {
	case err == nil:
		return nil
	case grpc.Code(err) == codes.Unknown:
		return grpcutil.Internal
	default:
		return err
	}
}

type registerStreamMutation struct {
	*coordinator.LogStream

	req          *logdog.RegisterStreamRequest
	archiveDelay time.Duration
}

func (m registerStreamMutation) RollForward(c context.Context) ([]tumble.Mutation, error) {
	di := ds.Get(c)

	// Already registered? (Transactional).
	switch err := di.Get(m.LogStream); err {
	case ds.ErrNoSuchEntity:
		break

	case nil:
		// The stream is already registered.
		//
		// We want this to be idempotent, so validate that it matches the current
		// configuration and return accordingly.
		if err := matchesLogStream(m.req, m.LogStream); err != nil {
			return nil, grpcutil.Errf(codes.AlreadyExists, "Log stream is already incompatibly registered (T): %v", err)
		}
		return nil, nil

	default:
		return nil, grpcutil.Internal
	}

	log.Infof(c, "Registering new log stream'")

	// The stream is not yet registered.
	if err := m.LoadDescriptor(m.req.Desc); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to load descriptor into LogStream.")
		return nil, grpcutil.Errf(codes.InvalidArgument, "Failed to load descriptor.")
	}

	m.Secret = m.req.Secret
	m.ProtoVersion = m.req.ProtoVersion
	m.State = coordinator.LSStreaming
	m.Created = ds.RoundTime(clock.Now(c).UTC())
	m.TerminalIndex = -1

	if err := di.Put(m.LogStream); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to Put() LogStream.")
		return nil, grpcutil.Internal
	}

	// Add a named delayed mutation to archive this stream if it's not archived
	// yet. We will cancel this in terminateStream once we dispatch an immediate
	// archival task.
	archiveExpiredMutation := mutations.CreateArchiveTask{
		Path:       m.Path(),
		Expiration: clock.Now(c).Add(m.archiveDelay),
	}
	log.Fields{
		"archiveDelay": m.archiveDelay,
		"deadline":     archiveExpiredMutation.Expiration,
	}.Infof(c, "Scheduling expiration deadline mutation.")
	aeParent, aeName := archiveExpiredMutation.TaskName(di)
	err := tumble.PutNamedMutations(c, aeParent, map[string]tumble.Mutation{
		aeName: &archiveExpiredMutation,
	})
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to load named mutations.")
		return nil, grpcutil.Internal
	}

	return []tumble.Mutation{
		&mutations.PutHierarchyMutation{
			Path: m.Path(),
		},
	}, nil
}

func (m registerStreamMutation) Root(c context.Context) *ds.Key {
	return ds.Get(c).KeyForObj(m.LogStream)
}
