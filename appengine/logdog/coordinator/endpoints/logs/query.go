// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logs

import (
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/grpcutil"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

const (
	// queryResultLimit is the maximum number of log streams that will be
	// returned in a single query. If the user requests more, it will be
	// automatically called at this value.
	queryResultLimit = 500
)

// applyTrinary executes the supplied query modification function based on a
// trinary value.
//
// If the value is "YES", it will be executed with "true". If "NO", "false". If
// "BOTH", it will not be executed.
func applyTrinary(q *ds.Query, v logdog.QueryRequest_Trinary, f func(*ds.Query, bool) *ds.Query) *ds.Query {
	switch v {
	case logdog.QueryRequest_YES:
		return f(q, true)

	case logdog.QueryRequest_NO:
		return f(q, false)

	default:
		// Default is "both".
		return q
	}
}

// Query returns log stream paths that match the requested query.
func (s *Server) Query(c context.Context, req *logdog.QueryRequest) (*logdog.QueryResponse, error) {
	svc := s.GetServices()

	// Non-admin users may not request purged results.
	canSeePurged := true
	if err := coordinator.IsAdminUser(c, svc); err != nil {
		canSeePurged = false

		// Non-admin user.
		if req.Purged == logdog.QueryRequest_YES {
			log.Fields{
				log.ErrorKey: err,
			}.Errorf(c, "Non-superuser requested to see purged logs. Denying.")
			return nil, grpcutil.Errf(codes.InvalidArgument, "non-admin user cannot request purged log streams")
		}
	}

	// Scale the maximum number of results based on the number of queries in this
	// request. If the user specified a maximum result count of zero, use the
	// default maximum.
	//
	// If this scaling results in a limit that is <1 per request, we will return
	// back a BadRequest error.
	limit := s.resultLimit
	if limit == 0 {
		limit = queryResultLimit
	}

	// Execute our queries in parallel.
	resp := logdog.QueryResponse{}
	e := &queryRunner{
		Context:      log.SetField(c, "path", req.Path),
		QueryRequest: req,
		canSeePurged: canSeePurged,
		limit:        limit,
	}
	if err := e.runQuery(&resp); err != nil {
		// Transient errors would be handled at the "execute" level, so these are
		// specific failure errors. We must escalate individual errors to the user.
		// We will choose the most severe of the resulting errors.
		log.WithError(err).Errorf(c, "Failed to execute query.")
		return nil, err
	}
	return &resp, nil
}

type queryRunner struct {
	context.Context
	*logdog.QueryRequest

	canSeePurged bool
	limit        int
}

func (r *queryRunner) runQuery(resp *logdog.QueryResponse) error {
	if r.limit == 0 {
		return grpcutil.Errf(codes.InvalidArgument, "query limit is zero")
	}

	if int(r.MaxResults) > 0 && r.limit > int(r.MaxResults) {
		r.limit = int(r.MaxResults)
	}

	q := ds.NewQuery("LogStream").Order("-Created")

	// Determine which entity to query against based on our sorting constraints.
	if r.Next != "" {
		cursor, err := ds.Get(r).DecodeCursor(r.Next)
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
				"cursor":     r.Next,
			}.Errorf(r, "Failed to decode cursor.")
			return grpcutil.Errf(codes.InvalidArgument, "invalid `next` value")
		}
		q = q.Start(cursor)
	}

	// Add Path constraints.
	if r.Path != "" {
		err := error(nil)
		q, err = coordinator.AddLogStreamPathFilter(q, r.Path)
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
				"path":       r.Path,
			}.Errorf(r, "Invalid query path.")
			return grpcutil.Errf(codes.InvalidArgument, "invalid query `path`")
		}
	}

	if r.ContentType != "" {
		q = q.Eq("ContentType", r.ContentType)
	}

	if st := r.StreamType; st != nil {
		switch v := st.Value; v {
		case logpb.StreamType_TEXT, logpb.StreamType_BINARY, logpb.StreamType_DATAGRAM:
			q = q.Eq("StreamType", v)

		default:
			return grpcutil.Errf(codes.InvalidArgument, "invalid query `streamType`: %s", v.String())
		}
	}

	q = applyTrinary(q, r.Terminated, coordinator.AddLogStreamTerminatedFilter)
	q = applyTrinary(q, r.Archived, coordinator.AddLogStreamArchivedFilter)

	if !r.canSeePurged {
		// Force non-purged results for non-admin users.
		q = q.Eq("Purged", false)
	} else {
		q = applyTrinary(q, r.Purged, coordinator.AddLogStreamPurgedFilter)
	}

	if r.Newer != nil {
		q = coordinator.AddNewerFilter(q, r.Newer.Time())
	}
	if r.Older != nil {
		q = coordinator.AddOlderFilter(q, r.Older.Time())
	}

	if r.ProtoVersion != "" {
		q = q.Eq("ProtoVersion", r.ProtoVersion)
	}

	// Add tag constraints.
	for k, v := range r.Tags {
		if err := types.ValidateTag(k, v); err != nil {
			log.Fields{
				log.ErrorKey: err,
				"key":        k,
				"value":      v,
			}.Errorf(r, "Invalid tag constraint.")
			return grpcutil.Errf(codes.InvalidArgument, "invalid tag constraint: %q", k)
		}
		q = coordinator.AddLogStreamTagFilter(q, k, v)
	}

	q = q.Limit(int32(r.limit))

	// Issue the query.
	if log.IsLogging(r, log.Debug) {
		fq, _ := q.Finalize()
		log.Fields{
			"query": fq.String(),
		}.Debugf(r, "Issuing query.")
	}

	cursor := ds.Cursor(nil)
	streams := make([]*logdog.QueryResponse_Stream, 0, r.limit)
	err := ds.Get(r).Run(q, func(ls *coordinator.LogStream, cb ds.CursorCB) error {
		stream := logdog.QueryResponse_Stream{
			Path: string(ls.Path()),
		}
		if r.State {
			stream.State = loadLogStreamState(ls)

			var err error
			stream.Desc, err = ls.DescriptorValue()
			if err != nil {
				return grpcutil.Internal
			}
		}
		streams = append(streams, &stream)

		// If we hit our limit, add a cursor for the next iteration.
		if len(streams) == r.limit {
			var err error
			cursor, err = cb()
			if err != nil {
				log.Fields{
					log.ErrorKey: err,
					"count":      len(streams),
				}.Errorf(r, "Failed to get cursor value.")
				return err
			}
			return ds.Stop
		}
		return nil
	})
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(r, "Failed to execute query.")
		return grpcutil.Internal
	}

	resp.Streams = streams
	if cursor != nil {
		resp.Next = cursor.String()
	}

	return nil
}
