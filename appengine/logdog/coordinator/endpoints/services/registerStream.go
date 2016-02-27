// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"crypto/subtle"
	"errors"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/mutations"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/grpcutil"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

func matchesLogStream(r *services.RegisterStreamRequest, ls *coordinator.LogStream) error {
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

func loadLogStreamState(ls *coordinator.LogStream) *services.LogStreamState {
	st := services.LogStreamState{
		Path:          string(ls.Path()),
		Secret:        ls.Secret,
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
func (b *Server) RegisterStream(c context.Context, req *services.RegisterStreamRequest) (*services.LogStreamState, error) {
	if err := Auth(c); err != nil {
		return nil, err
	}

	path := types.StreamPath(req.Path)
	if err := path.Validate(); err != nil {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Invalid path (%s): %s", path, err)
	}
	c = log.SetField(c, "path", path)

	if req.ProtoVersion == "" {
		return nil, grpcutil.Errf(codes.InvalidArgument, "No protobuf version supplied.")
	}
	if req.ProtoVersion != logpb.Version {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Unrecognized protobuf version.")
	}

	if len(req.Secret) != types.StreamSecretLength {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Invalid secret length (%d != %d)",
			len(req.Secret), types.StreamSecretLength)
	}

	// Validate our descriptor.
	if req.Desc == nil {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Missing log stream descriptor.")
	}
	prefix, name := path.Split()
	if err := req.Desc.Validate(true); err != nil {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Invalid log stream descriptor: %s", err)
	}
	if req.Desc.Prefix != string(prefix) {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Descriptor prefix does not match path (%s != %s)",
			req.Desc.Prefix, prefix)
	}
	if req.Desc.Name != string(name) {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Descriptor name does not match path (%s != %s)",
			req.Desc.Name, name)
	}

	// Already registered? (Non-transactional).
	ls := coordinator.LogStreamFromPath(path)
	if err := ds.Get(c).Get(ls); err == nil {
		// We want this to be idempotent, so validate that it matches the current
		// configuration and return accordingly.
		if err := matchesLogStream(req, ls); err != nil {
			return nil, grpcutil.Errf(codes.AlreadyExists, "Log stream is already incompatibly registered: %v", err)
		}

		// Return the current stream state.
		return loadLogStreamState(ls), nil
	}

	// The registration is valid, so retain it.
	if err := tumble.RunMutation(c, &registerStreamMutation{ls, req}); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to register LogStream.")
		return nil, filterError(err)
	}

	return loadLogStreamState(ls), nil
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

	req *services.RegisterStreamRequest
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

	now := ds.RoundTime(clock.Now(c).UTC())
	m.Secret = m.req.Secret
	m.ProtoVersion = m.req.ProtoVersion
	m.State = coordinator.LSPending
	m.Created = now
	m.Updated = now
	m.TerminalIndex = -1

	if err := m.Put(di); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to Put() LogStream.")
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
