// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"bytes"
	"crypto/subtle"
	"errors"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/golang/protobuf/proto"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/ephelper"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	lep "github.com/luci/luci-go/appengine/logdog/coordinator/endpoints"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logdog/protocol"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// RegisterStreamRequest is the set of caller-supplied data for the
// RegisterStream Coordinator service endpoint.
type RegisterStreamRequest struct {
	// Path is the log stream's path.
	Path string `json:"path,omitempty" endpoints:"req"`
	// Secret is the log stream's secret.
	Secret []byte `json:"secret,omitempty" endpoints:"req"`

	// ProtoVersion is the protobuf version string for this stream.
	ProtoVersion string `json:"protoVersion,omitempty" endpoints:"req"`
	// Descriptor is the serialized LogStreamDescriptor protobuf for this stream.
	Descriptor []byte `json:"descriptor,omitempty" endpoints:"req"`
}

func (r *RegisterStreamRequest) matchesLogStream(ls *coordinator.LogStream) error {
	if r.Path != string(ls.Path()) {
		return errors.New("paths do not match")
	}

	if subtle.ConstantTimeCompare(r.Secret, ls.Secret) != 1 {
		return errors.New("secrets do not match")
	}

	if r.ProtoVersion != ls.ProtoVersion {
		return errors.New("protobuf version does not match")
	}

	if !bytes.Equal(r.Descriptor, ls.Descriptor) {
		return errors.New("descriptor protobufs do not match")
	}

	return nil
}

// RegisterStreamResponse is the set of caller-supplied data for the
// RegisterStream Coordinator service endpoint.
type RegisterStreamResponse struct {
	// Path is the log stream's path.
	Path string `json:"path,omitempty"`
	// Secret is the log stream's secret.
	//
	// Note that the secret is returned! This is okay, since this endpoint is only
	// accessible to trusted services. The secret can be cached by services to
	// validate stream information without needing to ping the Coordinator in
	// between each update.
	Secret []byte `json:"secret,omitempty"`

	// Stream is the Coordinator's log stream state value.
	State *lep.LogStreamState `json:"state,omitempty"`
}

// RegisterStream is an idempotent stream state register operation.
func (s *Service) RegisterStream(c context.Context, req *RegisterStreamRequest) (*RegisterStreamResponse, error) {
	c, err := s.Use(c, MethodInfoMap["RegisterStream"])
	if err != nil {
		return nil, err
	}
	if err := Auth(c); err != nil {
		return nil, err
	}

	path := types.StreamPath(req.Path)
	if err := path.Validate(); err != nil {
		return nil, endpoints.NewBadRequestError("Invalid path (%s): %s", path, err)
	}
	c = log.SetField(c, "path", path)

	if req.ProtoVersion == "" {
		return nil, endpoints.NewBadRequestError("No protobuf version supplied.")
	}
	if req.ProtoVersion != protocol.Version {
		return nil, endpoints.NewBadRequestError("Unrecognized protobuf version.")
	}

	if len(req.Secret) != types.StreamSecretLength {
		return nil, endpoints.NewBadRequestError("Invalid secret length (%d != %d)",
			len(req.Secret), types.StreamSecretLength)
	}

	if req.Descriptor == nil {
		return nil, endpoints.NewBadRequestError("Missing log stream descriptor.")
	}

	// Validate our descriptor.
	prefix, name := path.Split()
	desc := protocol.LogStreamDescriptor{}
	if err := proto.Unmarshal(req.Descriptor, &desc); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"size":       len(req.Descriptor),
		}.Errorf(c, "Failed to unmarshal Descriptor protobuf.")
		return nil, endpoints.NewBadRequestError("Could not unmarshal Descriptor protobuf.")
	}
	if err := desc.Validate(true); err != nil {
		return nil, endpoints.NewBadRequestError("Invalid log stream descriptor: %s", err)
	}
	if desc.Prefix != string(prefix) {
		return nil, endpoints.NewBadRequestError("Descriptor prefix does not match path (%s != %s)",
			desc.Prefix, prefix)
	}
	if desc.Name != string(name) {
		return nil, endpoints.NewBadRequestError("Descriptor name does not match path (%s != %s)",
			desc.Name, name)
	}

	// Already registered? (Non-transactional).
	ls, err := coordinator.NewLogStream(req.Path)
	impossible(err)

	if err := ds.Get(c).Get(ls); err == nil {
		// We want this to be idempotent, so validate that it matches the current
		// configuration and return accordingly.
		if err := req.matchesLogStream(ls); err != nil {
			return nil, endpoints.NewConflictError("log stream is already incompatibly registered: %v", err)
		}

		// Return the current stream state.
		return &RegisterStreamResponse{
			Path:   string(path),
			Secret: ls.Secret,
			State:  lep.LoadLogStreamState(ls),
		}, nil
	}

	// The registration is valid, so retain it.
	now := coordinator.NormalizeTime(clock.Now(c).UTC())

	err = ds.Get(c).RunInTransaction(func(c context.Context) error {
		di := ds.Get(c)

		// Already registered? (Transactional).
		switch err := di.Get(ls); err {
		case nil:
			// The stream is already registered.
			//
			// We want this to be idempotent, so validate that it matches the current
			// configuration and return accordingly.
			if err := req.matchesLogStream(ls); err != nil {
				return endpoints.NewBadRequestError("log stream is already incompatibly registered (T): %v", err)
			}
			return nil

		case ds.ErrNoSuchEntity:
			log.Infof(c, "Registering new log stream'")

			// The stream is not yet registered.
			if err := ls.LoadDescriptor(&desc); err != nil {
				log.Fields{
					log.ErrorKey: err,
				}.Errorf(c, "Failed to load descriptor into LogStream.")
				return endpoints.NewBadRequestError("Failed to load descriptor.")
			}

			ls.Secret = req.Secret
			ls.ProtoVersion = req.ProtoVersion
			ls.State = coordinator.LSPending
			ls.Created = now
			ls.Updated = now
			ls.TerminalIndex = -1

			if err := ls.Put(di); err != nil {
				log.Fields{
					log.ErrorKey: err,
				}.Errorf(c, "Failed to Put() LogStream.")
				return err
			}

		default:
			return err
		}

		return nil
	}, &ds.TransactionOptions{XG: true})
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to update LogStream.")
		return nil, ephelper.StripError(err)
	}

	resp := RegisterStreamResponse{
		Path:   string(path),
		Secret: ls.Secret,
		State:  lep.LoadLogStreamState(ls),
	}
	return &resp, nil
}
