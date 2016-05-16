// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"time"

	"github.com/golang/protobuf/proto"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/hierarchy"
	"github.com/luci/luci-go/appengine/logdog/coordinator/mutations"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/grpcutil"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

func buildLogStreamState(ls *coordinator.LogStream, lst *coordinator.LogStreamState) *logdog.LogStreamState {
	st := logdog.LogStreamState{
		ProtoVersion:  ls.ProtoVersion,
		Secret:        lst.Secret,
		TerminalIndex: lst.TerminalIndex,
		Archived:      lst.ArchivalState().Archived(),
		Purged:        ls.Purged,
	}
	if !lst.Terminated() {
		st.TerminalIndex = -1
	}
	return &st
}

// RegisterStream is an idempotent stream state register operation.
//
// Successive operations will succeed if they have the correct secret for their
// registered stream, regardless of whether the contents of their request match
// the currently registered state.
func (s *server) RegisterStream(c context.Context, req *logdog.RegisterStreamRequest) (*logdog.RegisterStreamResponse, error) {
	var path types.StreamPath

	// Unmarshal the serialized protobuf.
	var desc logpb.LogStreamDescriptor
	switch req.ProtoVersion {
	case logpb.Version:
		if err := proto.Unmarshal(req.Desc, &desc); err != nil {
			log.Fields{
				log.ErrorKey:   err,
				"protoVersion": req.ProtoVersion,
			}.Errorf(c, "Failed to unmarshal descriptor protobuf.")
			return nil, grpcutil.Errf(codes.InvalidArgument, "Failed to unmarshal protobuf.")
		}

	default:
		log.Fields{
			"protoVersion": req.ProtoVersion,
		}.Errorf(c, "Unrecognized protobuf version.")
		return nil, grpcutil.Errf(codes.InvalidArgument, "Unrecognized protobuf version: %q", req.ProtoVersion)
	}

	path = desc.Path()
	log.Fields{
		"project": req.Project,
		"path":    path,
	}.Infof(c, "Registration request for log stream.")

	if err := desc.Validate(true); err != nil {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Invalid log stream descriptor: %s", err)
	}
	prefix, _ := path.Split()

	// Load our config and archive expiration.
	cfg, err := coordinator.GetServices(c).Config(c)
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

	// Register our Prefix.
	//
	// This will also verify that our request secret matches the registered one,
	// if one is registered.
	//
	// Note: This step will not be necessary once a "register prefix" RPC call is
	// implemented.
	lsp := logStreamPrefix{
		prefix: string(prefix),
		secret: req.Secret,
	}
	pfx, err := registerPrefix(c, &lsp)
	if err != nil {
		log.Errorf(c, "Failed to register/validate log stream prefix.")
		return nil, err
	}
	log.Fields{
		"prefix":        pfx.Prefix,
		"prefixCreated": pfx.Created,
	}.Debugf(c, "Loaded log stream prefix.")

	di := ds.Get(c)
	ls := &coordinator.LogStream{ID: coordinator.LogStreamID(path)}
	lst := ls.State(di)

	// Check for registration (non-transactional).
	if err := checkRegisterStream(di, ls, lst); err != nil {
		if !anyNoSuchEntity(err) {
			log.WithError(err).Errorf(c, "Failed to check for log stream.")
			return nil, err
		}

		// The stream is not registered. Perform a transactional registration via
		// mutation.
		//
		// Determine which hierarchy components we need to add.
		comps := hierarchy.Components(path)
		if comps, err = hierarchy.Missing(di, comps); err != nil {
			log.WithError(err).Warningf(c, "Failed to probe for missing hierarchy components.")
		}

		// Before we go into transaction, try and put these entries. This should not
		// be contested, since components don't share an entity root.
		if err := hierarchy.PutMulti(di, comps); err != nil {
			log.WithError(err).Infof(c, "Failed to add missing hierarchy components.")
			return nil, grpcutil.Internal
		}

		// The stream does not exist. Proceed with transactional registration.
		err = tumble.RunMutation(c, &registerStreamMutation{
			RegisterStreamRequest: req,
			desc:         &desc,
			pfx:          pfx,
			lst:          lst,
			ls:           ls,
			archiveDelay: archiveDelayMax,
		})
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
			}.Errorf(c, "Failed to register LogStream.")
			return nil, err
		}
	}

	return &logdog.RegisterStreamResponse{
		Id:    string(ls.ID),
		State: buildLogStreamState(ls, lst),
	}, nil
}

func checkRegisterStream(di ds.Interface, ls *coordinator.LogStream, lst *coordinator.LogStreamState) error {
	// Load the existing log stream state.
	//
	// We have already verified that the secrets match when the Prefix was
	// registered. This will have to verify secrets when prefix registration
	// moves its own RPC.
	return di.GetMulti([]interface{}{ls, lst})
}

type registerStreamMutation struct {
	*logdog.RegisterStreamRequest

	desc         *logpb.LogStreamDescriptor
	pfx          *coordinator.LogPrefix
	ls           *coordinator.LogStream
	lst          *coordinator.LogStreamState
	archiveDelay time.Duration
}

func (m *registerStreamMutation) RollForward(c context.Context) ([]tumble.Mutation, error) {
	di := ds.Get(c)

	// Check if our stream is registered (transactional).
	if err := checkRegisterStream(di, m.ls, m.lst); err != nil {
		if !anyNoSuchEntity(err) {
			log.WithError(err).Errorf(c, "Failed to check for stream registration (transactional).")
			return nil, err
		}

		// The stream is not yet registered.
		log.Infof(c, "Registering new log stream.")

		now := clock.Now(c).UTC()

		// Construct our LogStreamState.
		m.lst.Created = now
		m.lst.Updated = now
		m.lst.Secret = m.pfx.Secret // Copy Prefix Secret to reduce datastore Gets.
		m.lst.TerminalIndex = -1

		// Construct our LogStream.
		m.ls.Created = now
		m.ls.ProtoVersion = m.ProtoVersion

		if err := m.ls.LoadDescriptor(m.desc); err != nil {
			log.Fields{
				log.ErrorKey: err,
			}.Errorf(c, "Failed to load descriptor into LogStream.")
			return nil, grpcutil.Errf(codes.InvalidArgument, "Failed to load descriptor.")
		}

		if err := di.PutMulti([]interface{}{m.ls, m.lst}); err != nil {
			log.Fields{
				log.ErrorKey: err,
			}.Errorf(c, "Failed to Put LogStream.")
			return nil, grpcutil.Internal
		}

		// Add a named delayed mutation to archive this stream if it's not archived
		// yet. We will cancel this in terminateStream once we dispatch an immediate
		// archival task.
		archiveExpiredMutation := mutations.CreateArchiveTask{
			ID:         m.ls.ID,
			Expiration: now.Add(m.archiveDelay),
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
	}

	return nil, nil
}

func (m *registerStreamMutation) Root(c context.Context) *ds.Key {
	return ds.Get(c).KeyForObj(m.ls)
}
