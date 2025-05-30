// Copyright 2015 The LUCI Authors.
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

package logs

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth/realms"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/flex"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/types"
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

	// Fetch the stream metadata, including its state.
	fetcher := coordinator.MetadataFetcher{
		Path:      types.StreamPath(req.Path),
		WithState: true,
	}
	if err := fetcher.FetchWithACLCheck(c); err != nil {
		return nil, err
	}

	// If this log entry is Purged and we're not admin, pretend it doesn't exist.
	if fetcher.Stream.Purged {
		switch yes, err := coordinator.CheckAdminUser(c); {
		case err != nil:
			return nil, status.Error(codes.Internal, "internal server error")
		case !yes:
			log.Warningf(c, "Non-superuser requested purged log.")
			return nil, status.Errorf(codes.NotFound, "path not found")
		}
	}

	resp := logdog.GetResponse{Project: req.Project}
	if fetcher.Prefix.Realm != "" {
		_, resp.Realm = realms.Split(fetcher.Prefix.Realm)
	}

	if req.State {
		resp.State = buildLogStreamState(fetcher.Stream, fetcher.State)

		var err error
		resp.Desc, err = fetcher.Stream.DescriptorProto()
		if err != nil {
			log.WithError(err).Errorf(c, "Failed to deserialize descriptor protobuf.")
			return nil, status.Error(codes.Internal, "internal server error")
		}
	}

	// Retrieve requested logs from storage, if requested.
	startTime := clock.Now(c)
	if err := s.getLogs(c, req, &resp, tail, fetcher.Stream, fetcher.State); err != nil {
		log.WithError(err).Errorf(c, "Failed to get logs.")
		return nil, status.Error(codes.Internal, "internal server error")
	}

	log.Fields{
		"duration": clock.Now(c).Sub(startTime).String(),
		"logCount": len(resp.Logs),
	}.Debugf(c, "Get request completed successfully.")
	return &resp, nil
}

func (s *server) getLogs(c context.Context, req *logdog.GetRequest, resp *logdog.GetResponse,
	tail bool, ls *coordinator.LogStream, lst *coordinator.LogStreamState) error {

	// Identify our URL signing parameters.
	var signingRequest coordinator.URLSigningRequest
	if sr := req.GetSignedUrls; sr != nil {
		signingRequest.Lifetime = sr.Lifetime.AsDuration()
		signingRequest.Stream = sr.Stream
		signingRequest.Index = sr.Index
	}
	if !tail && req.LogCount < 0 && !signingRequest.HasWork() {
		// No log operations are actually needed, so don't bother instanting our
		// Storage instance only to do nothing.
		return nil
	}

	svc := flex.GetServices(c)
	st, err := svc.StorageForStream(c, lst, coordinator.Project(c))
	if err != nil {
		return errors.Fmt("failed to create storage instance: %w", err)
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
			return errors.Fmt("failed to generate signed URL: %w", err)

		case signedURLs == nil:
			log.Debugf(c, "Signed URL was requested, but is not supported by storage.")

		default:
			resp.SignedUrls = &logdog.GetResponse_SignedUrls{
				Expiration: timestamppb.New(signedURLs.Expiration),
				Stream:     signedURLs.Stream,
				Index:      signedURLs.Index,
			}
		}
	}

	return nil
}

func getHead(c context.Context, req *logdog.GetRequest, st coordinator.SigningStorage, project string,
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
	err := retry.Retry(c, transient.Only(retry.Default), func() error {
		// Issue the Get request. This may return a transient error, in which case
		// we will retry.
		return st.Get(c, sreq, func(e *storage.Entry) bool {
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

func getTail(c context.Context, st coordinator.SigningStorage, project string, path types.StreamPath) (
	[]*logpb.LogEntry, error) {

	log.Fields{
		"project": project,
		"path":    path,
	}.Debugf(c, "Issuing Tail request.")

	var e *storage.Entry
	err := retry.Retry(c, transient.Only(retry.Default), func() (err error) {
		e, err = st.Tail(c, project, path)
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
