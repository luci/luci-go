// Copyright 2018 The LUCI Authors.
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

package view

import (
	"io"
	"net/http"
	"strings"
	"time"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/flex"
	"go.chromium.org/luci/logdog/common/fetcher"
	"go.chromium.org/luci/logdog/common/renderer"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/types"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/router"

	ds "go.chromium.org/gae/service/datastore"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
)

// HandleTextLogs is a router.Handler that streams log request data on demand.
//
// HandleTextLogs requires LogDog Services to be installed into the Context.
func HandleTextLogs(c *router.Context) {
	if err := handleTextLogsImpl(c.Context, c.Request, c.Writer); err != nil {
		errors.Log(c.Context, err)
		http.Error(c.Writer, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// handleTextLogsImpl handles a log text request.
//
// handleTextLogsImpl will return an error only for internal errors;
// otherwise, it will handle the error itself using rw.
func handleTextLogsImpl(c context.Context, req *http.Request, rw http.ResponseWriter) error {
	// TODO: Handle authentication. How? Probably would use a one-time token
	// generated with the link, or have self-signed authenticated links, or
	// something.
	q := req.URL.Query()
	project, path, err := parseProjectAndPath(q.Get("p"))
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return nil
	}
	if err := coordinator.WithProjectNamespace(&c, project, coordinator.NamespaceAccessREAD); err != nil {
		return err
	}

	ls := &coordinator.LogStream{ID: coordinator.LogStreamID(path)}
	lst := ls.State(c)
	logging.Fields{
		"id": ls.ID,
	}.Debugf(c, "Loading stream.")

	if err := ds.Get(c, ls, lst); err != nil {
		if isNoSuchEntity(err) {
			logging.Warningf(c, "Log stream does not exist.")
			http.NotFound(rw, req)
			return nil
		}

		return errors.Annotate(err, "failed to look up log stream").Err()
	}

	// Load the log stream descriptor protobuf.
	desc, err := ls.DescriptorValue()
	if err != nil {
		return errors.Annotate(err, "failed to unmarshal descriptor").Err()
	}

	svc := flex.GetServices(c)
	st, err := svc.StorageForStream(c, lst)
	if err != nil {
		return errors.Annotate(err, "").InternalReason("failed to create storage instance").Err()
	}
	defer st.Close()

	// Ensure that we cancel our Fetcher when we exit.
	c, cancelFunc := context.WithCancel(c)
	defer cancelFunc()

	fs := fetcherSource{
		ls: ls,
		st: st,
		sreq: storage.GetRequest{
			Project: project,
			Path:    path,
		},
		desc: desc,
		tidx: -1,
	}
	f := fetcher.New(c, fetcher.Options{
		Source: &fs,
	})

	rend := renderer.Renderer{
		Source:         f,
		Raw:            false,
		DatagramWriter: getDatagramWriter(c, desc),
	}
	amt, err := io.Copy(rw, &rend)
	if err != nil {
		return errors.Annotate(err, "failed to write log data").Err()
	}

	logging.Fields{
		"bytes": amt,
	}.Infof(c, "Finished rendering request.")
	return nil
}

// getDatagramWriter returns a datagram writer function that can be used as a
// Renderer's DatagramWriter. The writer is bound to desc.
func getDatagramWriter(c context.Context, desc *logpb.LogStreamDescriptor) renderer.DatagramWriter {

	return func(w io.Writer, dg []byte) bool {
		var pb proto.Message
		switch desc.ContentType {
		case milo.ContentTypeAnnotations:
			mp := milo.Step{}
			if err := proto.Unmarshal(dg, &mp); err != nil {
				logging.WithError(err).Errorf(c, "Failed to unmarshal datagram data.")
				return false
			}
			pb = &mp

		default:
			return false
		}

		if err := proto.MarshalText(w, pb); err != nil {
			logging.WithError(err).Errorf(c, "Failed to marshal datagram as text.")
			return false
		}

		return true
	}
}

type fetcherSource struct {
	ls *coordinator.LogStream
	st storage.Storage

	sreq storage.GetRequest
	desc *logpb.LogStreamDescriptor
	tidx types.MessageIndex
}

func (fs *fetcherSource) LogEntries(c context.Context, lr *fetcher.LogRequest) (
	[]*logpb.LogEntry, types.MessageIndex, error) {

	// If we don't have a terminal index yet, reload the log stream.
	if err := fs.maybeRefreshTerminalIndex(c); err != nil {
		return nil, fs.tidx, err
	}

	// Fetch the requested logs from our underlying storage.
	sreq := fs.sreq
	sreq.Limit = lr.Count
	sreq.Index = lr.Index

	logs, err := fs.runStorageRequest(c, sreq)
	return logs, fs.tidx, err
}

func (fs *fetcherSource) Descriptor() *logpb.LogStreamDescriptor { return fs.desc }

func (fs *fetcherSource) runStorageRequest(c context.Context, sreq storage.GetRequest) (
	[]*logpb.LogEntry, error) {

	// Allocate a return buffer to requested capacity.
	var logs []*logpb.LogEntry
	if sreq.Limit > 0 {
		logs = make([]*logpb.LogEntry, 0, sreq.Limit)
	}

	var ierr error
	err := retry.Retry(c, transient.Only(retry.Default), func() error {
		// Issue the Get request. This may return a transient error, in which case
		// we will retry.
		return fs.st.Get(c, sreq, func(e *storage.Entry) bool {
			var le *logpb.LogEntry
			if le, ierr = e.GetLogEntry(); ierr != nil {
				return false
			}

			logs = append(logs, le)
			return true
		})
	}, func(err error, delay time.Duration) {
		logging.Fields{
			logging.ErrorKey: err,
			"delay":          delay,
			"index":          sreq.Index,
			"limit":          sreq.Limit,
		}.Warningf(c, "Transient error while loading logs; retrying.")
	})
	switch err {
	case nil:
		if ierr != nil {
			logging.WithError(ierr).Errorf(c, "Bad log entry data.")
			return nil, ierr
		}
		return logs, nil

	case storage.ErrDoesNotExist:
		return nil, errors.New("log stream does not exist")

	default:
		logging.Fields{
			logging.ErrorKey: err,
			"index":          sreq.Index,
			"limit":          sreq.Limit,
		}.Errorf(c, "Failed to execute range request.")
		return nil, err
	}
}

func (fs *fetcherSource) maybeRefreshTerminalIndex(c context.Context) error {
	if fs.tidx >= 0 {
		return nil
	}

	lst := fs.ls.State(c)
	if err := ds.Get(c, lst); err != nil {
		return errors.Annotate(err, "failed to refresh log stream").Err()
	}
	fs.tidx = types.MessageIndex(lst.TerminalIndex)
	return nil
}

func parseProjectAndPath(v string) (project types.ProjectName, path types.StreamPath, err error) {
	if v == "" {
		err = errors.New("empty path parameter")
		return
	}

	parts := strings.SplitN(v, types.StreamNameSepStr, 2)
	if len(parts) != 2 {
		err = errors.New("missing project and path values")
		return
	}

	project, path = types.ProjectName(parts[0]), types.StreamPath(parts[1])
	if err = project.Validate(); err != nil {
		err = errors.Annotate(err, "invalid project name %s", project).Err()
		return
	}
	if err = path.Validate(); err != nil {
		err = errors.Annotate(err, "invalid log stream path %s", path).Err()
		return
	}
	return
}

func isNoSuchEntity(err error) bool {
	return errors.Any(err, func(err error) bool {
		return err == ds.ErrNoSuchEntity
	})
}
