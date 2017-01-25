// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package logs

import (
	"time"

	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/retry"
	"github.com/luci/luci-go/grpc/grpcutil"
	"github.com/luci/luci-go/logdog/api/endpoints/coordinator/logs/v1"
	"github.com/luci/luci-go/logdog/api/logpb"
	"github.com/luci/luci-go/logdog/appengine/coordinator"
	"github.com/luci/luci-go/logdog/common/storage"
	"github.com/luci/luci-go/logdog/common/types"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"

	ds "github.com/luci/gae/service/datastore"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

const (
	// getInitialArraySize is the initial amount of log slots to allocate for a
	// Get request.
	getInitialArraySize = 256

	// getBytesLimit is the maximum amount of data that we are willing to query.
	//
	// We will limit byte responses to 16MB, based on the following constraints:
	//	- AppEngine cannot respond with more than 32MB of data. This includes JSON
	//	  overhead, including notation and base64 data expansion.
	//	- `urlfetch`, which is used for Google Cloud Storage (archival) responses,
	//	  cannot handle responses larger than 32MB.
	getBytesLimit = 16 * 1024 * 1024
)

// Get returns state and log data for a single log stream.
func (s *server) Get(c context.Context, req *logdog.GetRequest) (*logdog.GetResponse, error) {
	return s.getImpl(c, req, false)
}

// Tail returns the last log entry for a given log stream.
func (s *server) Tail(c context.Context, req *logdog.TailRequest) (*logdog.GetResponse, error) {
	r := logdog.GetRequest{
		Project: req.Project,
		Path:    req.Path,
		State:   req.State,
	}
	return s.getImpl(c, &r, true)
}

// getImpl is common code shared between Get and Tail endpoints.
func (s *server) getImpl(c context.Context, req *logdog.GetRequest, tail bool) (*logdog.GetResponse, error) {
	log.Fields{
		"project": req.Project,
		"path":    req.Path,
		"index":   req.Index,
		"tail":    tail,
	}.Debugf(c, "Received get request.")

	path := types.StreamPath(req.Path)
	if err := path.Validate(); err != nil {
		log.WithError(err).Errorf(c, "Invalid path supplied.")
		return nil, grpcutil.Errf(codes.InvalidArgument, "invalid path value")
	}

	ls := &coordinator.LogStream{ID: coordinator.LogStreamID(path)}
	lst := ls.State(c)
	log.Fields{
		"id": ls.ID,
	}.Debugf(c, "Loading stream.")

	if err := ds.Get(c, ls, lst); err != nil {
		if isNoSuchEntity(err) {
			log.Errorf(c, "Log stream does not exist.")
			return nil, grpcutil.Errf(codes.NotFound, "path not found")
		}

		log.WithError(err).Errorf(c, "Failed to look up log stream.")
		return nil, grpcutil.Internal
	}

	// If this log entry is Purged and we're not admin, pretend it doesn't exist.
	if ls.Purged {
		if authErr := coordinator.IsAdminUser(c); authErr != nil {
			log.Fields{
				log.ErrorKey: authErr,
			}.Warningf(c, "Non-superuser requested purged log.")
			return nil, grpcutil.Errf(codes.NotFound, "path not found")
		}
	}

	resp := logdog.GetResponse{}
	if req.State {
		resp.State = buildLogStreamState(ls, lst)

		var err error
		resp.Desc, err = ls.DescriptorValue()
		if err != nil {
			log.WithError(err).Errorf(c, "Failed to deserialize descriptor protobuf.")
			return nil, grpcutil.Internal
		}
	}

	// Retrieve requested logs from storage, if requested.
	if err := s.getLogs(c, req, &resp, tail, ls, lst); err != nil {
		log.WithError(err).Errorf(c, "Failed to get logs.")
		return nil, grpcutil.Internal
	}

	log.Fields{
		"logCount": len(resp.Logs),
	}.Debugf(c, "Get request completed successfully.")
	return &resp, nil
}

func (s *server) getLogs(c context.Context, req *logdog.GetRequest, resp *logdog.GetResponse,
	tail bool, ls *coordinator.LogStream, lst *coordinator.LogStreamState) error {

	// Identify our URL signing parameters.
	var signingRequest coordinator.URLSigningRequest
	if sr := req.GetSignedUrls; sr != nil {
		signingRequest.Lifetime = google.DurationFromProto(sr.Lifetime)
		signingRequest.Stream = sr.Stream
		signingRequest.Index = sr.Index
	}
	if !tail && req.LogCount < 0 && !signingRequest.HasWork() {
		// No log operations are acutally needed, so don't bother instanting our
		// Storage instance only to do nothing.
		return nil
	}

	svc := coordinator.GetServices(c)
	st, err := svc.StorageForStream(c, lst)
	if err != nil {
		return errors.Annotate(err).InternalReason("failed to create storage instance").Err()
	}
	defer st.Close()

	project, path := coordinator.Project(c), ls.Path()

	if tail {
		resp.Logs, err = getTail(c, st, project, path)
	} else if req.LogCount >= 0 {
		byteLimit := int(req.ByteCount)
		if byteLimit <= 0 || byteLimit > getBytesLimit {
			byteLimit = getBytesLimit
		}

		resp.Logs, err = getHead(c, req, st, project, path, byteLimit)
	}
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to fetch log records.")
		return err
	}

	// If we're requesting a signedl URL, try and get that too.
	if signingRequest.HasWork() {
		signedURLs, err := st.GetSignedURLs(c, &signingRequest)
		switch {
		case err != nil:
			return errors.Annotate(err).InternalReason("failed to generate signed URL").Err()

		case signedURLs == nil:
			log.Debugf(c, "Signed URL was requested, but is not supported by storage.")

		default:
			resp.SignedUrls = &logdog.GetResponse_SignedUrls{
				Expiration: google.NewTimestamp(signedURLs.Expiration),
				Stream:     signedURLs.Stream,
				Index:      signedURLs.Index,
			}
		}
	}

	return nil
}

func getHead(c context.Context, req *logdog.GetRequest, st coordinator.Storage, project cfgtypes.ProjectName,
	path types.StreamPath, byteLimit int) ([]*logpb.LogEntry, error) {
	log.Fields{
		"project":       project,
		"path":          path,
		"index":         req.Index,
		"count":         req.LogCount,
		"bytes":         req.ByteCount,
		"noncontiguous": req.NonContiguous,
	}.Debugf(c, "Issuing Get request.")

	// Allocate result logs array.
	logCount := int(req.LogCount)
	asz := getInitialArraySize
	if logCount > 0 && logCount < asz {
		asz = logCount
	}
	logs := make([]*logpb.LogEntry, 0, asz)

	sreq := storage.GetRequest{
		Project: project,
		Path:    path,
		Index:   types.MessageIndex(req.Index),
		Limit:   logCount,
	}

	var ierr error
	count := 0
	err := retry.Retry(c, retry.TransientOnly(retry.Default), func() error {
		// Issue the Get request. This may return a transient error, in which case
		// we will retry.
		return st.Get(sreq, func(e *storage.Entry) bool {
			var le *logpb.LogEntry
			if le, ierr = e.GetLogEntry(); ierr != nil {
				return false
			}

			if count > 0 && byteLimit-len(e.D) < 0 {
				// Not the first log, and we've exceeded our byte limit.
				return false
			}

			sidx, _ := e.GetStreamIndex() // GetLogEntry succeeded, so this must.
			if !(req.NonContiguous || sidx == sreq.Index) {
				return false
			}

			logs = append(logs, le)
			sreq.Index = sidx + 1
			byteLimit -= len(e.D)
			count++
			return !(logCount > 0 && count >= logCount)
		})
	}, func(err error, delay time.Duration) {
		log.Fields{
			log.ErrorKey:   err,
			"delay":        delay,
			"initialIndex": req.Index,
			"nextIndex":    sreq.Index,
			"count":        len(logs),
		}.Warningf(c, "Transient error while loading logs; retrying.")
	})
	switch err {
	case nil:
		if ierr != nil {
			log.WithError(ierr).Errorf(c, "Bad log entry data.")
			return nil, ierr
		}
		return logs, nil

	case storage.ErrDoesNotExist:
		return nil, nil

	default:
		log.Fields{
			log.ErrorKey:   err,
			"initialIndex": req.Index,
			"nextIndex":    sreq.Index,
			"count":        len(logs),
		}.Errorf(c, "Failed to execute range request.")
		return nil, err
	}
}

func getTail(c context.Context, st coordinator.Storage, project cfgtypes.ProjectName, path types.StreamPath) (
	[]*logpb.LogEntry, error) {
	log.Fields{
		"project": project,
		"path":    path,
	}.Debugf(c, "Issuing Tail request.")

	var e *storage.Entry
	err := retry.Retry(c, retry.TransientOnly(retry.Default), func() (err error) {
		e, err = st.Tail(project, path)
		return
	}, func(err error, delay time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"delay":      delay,
		}.Warningf(c, "Transient error while fetching tail log; retrying.")
	})
	switch err {
	case nil:
		le, err := e.GetLogEntry()
		if err != nil {
			log.WithError(err).Errorf(c, "Failed to load tail entry data.")
			return nil, err
		}
		return []*logpb.LogEntry{le}, nil

	case storage.ErrDoesNotExist:
		return nil, nil

	default:
		log.WithError(err).Errorf(c, "Failed to fetch tail log.")
		return nil, err
	}
}
