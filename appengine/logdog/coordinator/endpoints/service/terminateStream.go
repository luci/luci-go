// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"crypto/subtle"
	"errors"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/ephelper"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

var errAlreadyUpdated = errors.New("already updated")

// TerminateStreamRequest is the set of caller-supplied data for the
// TerminateStream Coordinator service endpoint.
type TerminateStreamRequest struct {
	// Path is the log stream's path.
	Path string `json:"path,omitempty" endpoints:"req"`
	// Secret is the log stream's secret.
	Secret []byte `json:"secret,omitempty" endpoints:"req"`

	// TerminalIndex is the terminal index of the stream.
	TerminalIndex int64 `json:"terminalIndex,string"`
}

// TerminateStream is an idempotent stream state terminate operation.
func (s *Service) TerminateStream(c context.Context, req *TerminateStreamRequest) error {
	c, err := s.Use(c, MethodInfoMap["TerminateStream"])
	if err != nil {
		return err
	}
	if err := Auth(c); err != nil {
		return err
	}

	if err := types.StreamPath(req.Path).Validate(); err != nil {
		return endpoints.NewBadRequestError("Invalid path (%s): %s", req.Path, err)
	}
	c = log.SetField(c, "path", req.Path)

	if req.TerminalIndex < 0 {
		return endpoints.NewBadRequestError("Negative terminal index.")
	}

	// Initialize our log stream. This cannot fail since we have already validated
	// req.Path.
	ls, err := coordinator.NewLogStream(req.Path)
	impossible(err)

	// Can we update the index? (Non-transactional).
	switch err := updateTerminalIndex(c, ls, req); err {
	// To be confirmed/resolved transactionally.
	case nil:
		break
	case ds.ErrNoSuchEntity:
		break

	case errAlreadyUpdated:
		return nil

	default:
		return ephelper.StripError(err)
	}

	// Transactionally update.
	now := clock.Now(c).UTC()
	err = ds.Get(c).RunInTransaction(func(c context.Context) error {
		di := ds.Get(c)

		// Load the log stream state.
		switch updateTerminalIndex(c, ls, req) {
		case nil:
			ls.Updated = now
			ls.State = coordinator.LSTerminated
			if err := ls.Put(di); err != nil {
				log.Fields{
					log.ErrorKey: err,
				}.Errorf(c, "Failed to Put() LogStream.")
				return endpoints.NewInternalServerError("")
			}

			log.Fields{
				"terminalIndex": ls.TerminalIndex,
			}.Infof(c, "Terminal index was set.")
			return nil

		case errAlreadyUpdated:
			log.Fields{
				"terminalIndex": ls.TerminalIndex,
			}.Debugf(c, "Terminal index was already set.")
			return nil

		case ds.ErrNoSuchEntity:
			return endpoints.NewNotFoundError("log stream [%s] is not registered", req.Path)

		default:
			log.Fields{
				log.ErrorKey: err,
			}.Errorf(c, "Error retriving log stream.")
			return endpoints.NewInternalServerError("failed to retrieve log stream")
		}
	}, nil)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to update LogStream.")
		return ephelper.StripError(err)
	}

	return nil
}

func updateTerminalIndex(c context.Context, ls *coordinator.LogStream, req *TerminateStreamRequest) error {
	if err := ds.Get(c).Get(ls); err != nil {
		return err
	}

	if subtle.ConstantTimeCompare(ls.Secret, req.Secret) != 1 {
		return endpoints.NewBadRequestError("Request Secret doesn't match the stream secret.")
	}

	switch {
	case ls.TerminalIndex == req.TerminalIndex:
		// Idempotent: already updated to this value.
		return errAlreadyUpdated

	case ls.Terminated():
		// Idempotent: already updated to this value.
		return endpoints.NewConflictError("terminal index is already set")

	default:
		ls.TerminalIndex = req.TerminalIndex
		return nil
	}
}
