// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logs

import (
	"net/url"
	"time"

	"github.com/golang/protobuf/proto"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/grpcutil"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"github.com/luci/luci-go/common/retry"
	"github.com/luci/luci-go/server/logdog/storage"
	"github.com/luci/luci-go/server/logdog/storage/archive"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

const (
	// getInitialArraySize is the initial amount of log slots to allocate for a
	// Get request.
	getInitialArraySize = 256

	// getBytesLimit is the maximum amount of data that we are willing to query.
	// AppEngine limits our response size to 32MB. However, this limit applies
	// to the raw recovered LogEntry data, so we'll artificially constrain this
	// to 16MB so the additional JSON overhead doesn't kill it.
	getBytesLimit = 16 * 1024 * 1024
)

// Get returns state and log data for a single log stream.
func (s *Server) Get(c context.Context, req *logs.GetRequest) (*logs.GetResponse, error) {
	return s.getImpl(c, req, false)
}

// Tail returns the last log entry for a given log stream.
func (s *Server) Tail(c context.Context, req *logs.TailRequest) (*logs.GetResponse, error) {
	r := logs.GetRequest{
		Path:  req.Path,
		State: req.State,
	}
	return s.getImpl(c, &r, true)
}

// getImpl is common code shared between Get and Tail endpoints.
func (s *Server) getImpl(c context.Context, req *logs.GetRequest, tail bool) (*logs.GetResponse, error) {
	// Fetch the log stream state for this log stream.
	u, err := url.Parse(req.Path)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"path":       req.Path,
		}.Errorf(c, "Could not parse path URL.")
		return nil, grpcutil.Errf(codes.InvalidArgument, "invalid path encoding")
	}
	ls, err := coordinator.NewLogStream(u.Path)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"path":       u.Path,
		}.Errorf(c, "Invalid path supplied.")
		return nil, grpcutil.Errf(codes.InvalidArgument, "invalid path value")
	}

	// If this log entry is Purged and we're not admin, pretend it doesn't exist.
	err = ds.Get(c).Get(ls)
	switch err {
	case nil:
		if ls.Purged {
			if authErr := config.IsAdminUser(c); authErr != nil {
				log.Fields{
					log.ErrorKey: authErr,
				}.Warningf(c, "Non-superuser requested purged log.")
				return nil, grpcutil.Errf(codes.NotFound, "path not found")
			}
		}

	case ds.ErrNoSuchEntity:
		log.Fields{
			"path": u.Path,
		}.Errorf(c, "Log stream does not exist.")
		return nil, grpcutil.Errf(codes.NotFound, "path not found")

	default:
		log.Fields{
			log.ErrorKey: err,
			"path":       u.Path,
		}.Errorf(c, "Failed to look up log stream.")
		return nil, grpcutil.Internal
	}
	path := ls.Path()

	// If nothing was requested, return nothing.
	resp := logs.GetResponse{}
	if !(req.State || tail) && req.LogCount < 0 {
		return &resp, nil
	}

	if req.State {
		resp.State = loadLogStreamState(ls)

		var err error
		resp.Desc, err = ls.DescriptorValue()
		if err != nil {
			log.WithError(err).Errorf(c, "Failed to deserialize descriptor protobuf.")
			return nil, grpcutil.Internal
		}
	}

	// Retrieve requested logs from storage, if requested.
	if tail || req.LogCount >= 0 {
		resp.Logs, err = s.getLogs(c, req, tail, ls)
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
				"path":       path,
			}.Errorf(c, "Failed to get logs.")
			return nil, grpcutil.Internal
		}
	}

	log.Fields{
		"logCount": len(resp.Logs),
	}.Debugf(c, "Get request completed successfully.")
	return &resp, nil
}

func (s *Server) getLogs(c context.Context, req *logs.GetRequest, tail bool, ls *coordinator.LogStream) (
	[]*logpb.LogEntry, error) {
	var st storage.Storage
	if !ls.Archived() {
		log.Debugf(c, "Log is not archived. Fetching from intermediate storage.")

		// Logs are not archived. Fetch from intermediate storage.
		var err error
		st, err = s.Storage(c)
		if err != nil {
			return nil, err
		}
	} else {
		log.Debugf(c, "Log is archived. Fetching from archive storage.")
		var err error
		gs, err := s.GSClient(c)
		if err != nil {
			log.WithError(err).Errorf(c, "Failed to create Google Storage client.")
			return nil, err
		}
		defer func() {
			if err := gs.Close(); err != nil {
				log.WithError(err).Warningf(c, "Failed to close Google Storage client.")
			}
		}()

		st, err = archive.New(c, archive.Options{
			IndexURL:  ls.ArchiveIndexURL,
			StreamURL: ls.ArchiveStreamURL,
			Client:    gs,
		})
		if err != nil {
			log.WithError(err).Errorf(c, "Failed to create Google Storage storage instance.")
			return nil, err
		}
	}
	defer st.Close()

	path := ls.Path()

	var fetchedLogs [][]byte
	var err error
	if tail {
		fetchedLogs, err = getTail(c, st, path)
	} else {
		fetchedLogs, err = getHead(c, req, st, path)
	}
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to fetch log records.")
		return nil, err
	}

	logEntries := make([]*logpb.LogEntry, len(fetchedLogs))
	for idx, ld := range fetchedLogs {
		// Deserialize the log entry, then convert it to output value.
		le := logpb.LogEntry{}
		if err := proto.Unmarshal(ld, &le); err != nil {
			log.Fields{
				log.ErrorKey: err,
				"index":      idx,
			}.Errorf(c, "Failed to generate response log entry.")
			return nil, err
		}
		logEntries[idx] = &le
	}
	return logEntries, nil
}

func getHead(c context.Context, req *logs.GetRequest, st storage.Storage, p types.StreamPath) ([][]byte, error) {
	c = log.SetFields(c, log.Fields{
		"path":          p,
		"index":         req.Index,
		"count":         req.LogCount,
		"bytes":         req.ByteCount,
		"noncontiguous": req.NonContiguous,
	})

	byteLimit := int(req.ByteCount)
	if byteLimit <= 0 || byteLimit > getBytesLimit {
		byteLimit = getBytesLimit
	}

	// Allocate result logs array.
	logCount := int(req.LogCount)
	asz := getInitialArraySize
	if logCount > 0 && logCount < asz {
		asz = logCount
	}
	logs := make([][]byte, 0, asz)

	sreq := storage.GetRequest{
		Path:  p,
		Index: types.MessageIndex(req.Index),
		Limit: logCount,
	}

	count := 0
	err := retry.Retry(c, retry.TransientOnly(retry.Default), func() error {
		// Issue the Get request. This may return a transient error, in which case
		// we will retry.
		return st.Get(&sreq, func(idx types.MessageIndex, ld []byte) bool {
			if count > 0 && byteLimit-len(ld) < 0 {
				// Not the first log, and we've exceeded our byte limit.
				return false
			}
			byteLimit -= len(ld)

			if !(req.NonContiguous || idx == sreq.Index) {
				return false
			}
			logs = append(logs, ld)
			sreq.Index = idx + 1
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
	if err != nil {
		log.Fields{
			log.ErrorKey:   err,
			"initialIndex": req.Index,
			"nextIndex":    sreq.Index,
			"count":        len(logs),
		}.Errorf(c, "Failed to execute range request.")
		return nil, err
	}

	return logs, nil
}

func getTail(c context.Context, st storage.Storage, p types.StreamPath) ([][]byte, error) {
	var data []byte
	err := retry.Retry(c, retry.TransientOnly(retry.Default), func() (err error) {
		data, _, err = st.Tail(p)
		return
	}, func(err error, delay time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"delay":      delay,
		}.Warningf(c, "Transient error while fetching tail log; retrying.")
	})
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to fetch tail log.")
		return nil, err
	}
	return [][]byte{data}, err
}
