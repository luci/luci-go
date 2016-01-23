// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logs

import (
	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/ephelper"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	lep "github.com/luci/luci-go/appengine/logdog/coordinator/endpoints"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"golang.org/x/net/context"
)

const (
	// queryResultLimit is the maximum number of log streams that will be
	// returned in a single query. If the user requests more, it will be
	// automatically called at this value.
	queryResultLimit = 500
)

var (
	// streamTypeFilterMap maps StreamType query filter field strings to log
	// stream types.
	streamTypeFilterMap = map[string]logpb.LogStreamDescriptor_StreamType{
		"text":     logpb.LogStreamDescriptor_TEXT,
		"binary":   logpb.LogStreamDescriptor_BINARY,
		"datagram": logpb.LogStreamDescriptor_DATAGRAM,
	}
)

// TrinaryValue represents a trinary query value.
type TrinaryValue string

const (
	// TrinaryBoth means that both positive and negative results will be returned.
	TrinaryBoth TrinaryValue = "both"
	// TrinaryYes means that only positive results will be returned.
	TrinaryYes TrinaryValue = "yes"
	// TrinaryNo means that only negative results will be returned.
	TrinaryNo TrinaryValue = "no"
)

// apply adds a query equality filter for the specified field based on the
// TrinaryValue's value.
func (v TrinaryValue) apply(q *ds.Query, f func(*ds.Query, bool) *ds.Query) *ds.Query {
	switch v {
	case TrinaryYes:
		return f(q, true)

	case TrinaryNo:
		return f(q, false)

	default:
		// Default is "both".
		return q
	}
}

// QueryRequest is the request structure for the user Query endpoint.
type QueryRequest struct {
	// Path is the query parameter.
	//
	// The path expression may substitute a glob ("*") for a specific path
	// component. That is, any stream that matches the remaining structure qualifies
	// regardless of its value in that specific positional field.
	//
	// An unbounded wildcard may appear as a component at the end of both the
	// prefix and name query components. "**" matches all remaining components.
	//
	// If the supplied path query does not contain a path separator ("+"), it will
	// be treated as if the prefix is "**".
	//
	// Examples:
	//   - Empty ("") will return all streams.
	//   - **/+/** will return all streams.
	//   - foo/bar/** will return all streams with the "foo/bar" prefix.
	//   - foo/bar/**/+/baz will return all streams beginning with the "foo/bar"
	//     prefix and named "baz" (e.g., "foo/bar/qux/lol/+/baz")
	//   - foo/bar/+/** will return all streams with a "foo/bar" prefix.
	//   - foo/*/+/baz will return all streams with a two-component prefix whose
	//     first value is "foo" and whose name is "baz".
	//   - foo/bar will return all streams whose name is "foo/bar".
	//   - */* will return all streams with two-component names.
	Path string `json:"path,omitempty"`

	// ContentType, if not empty, restricts results to streams with the supplied
	// content type.
	ContentType string `json:"contentType,omitempty"`

	// StreamType is the stream type to filter on. This must be one of the values
	// in streamTypeFilterMap.
	StreamType string `json:"streamType,omitempty"`

	// Terminated, if not nil, restricts the query to streams that have or haven't
	// been terminated.
	//
	// TODO(dnj): Set endpoint default to "both" once the upconvert bug is fixed.
	// See https://github.com/GoogleCloudPlatform/go-endpoints/issues/139
	Terminated TrinaryValue `json:"terminated,omitempty"`
	// Archived, if not nil, restricts the query to streams that have or haven't
	// been archived.
	//
	// TODO(dnj): Set endpoint default to "both" once the upconvert bug is fixed.
	Archived TrinaryValue `json:"archived,omitempty"`
	// Purged, if not nil, restricts the query to streams that have or haven't
	// been purged.
	//
	// TODO(dnj): Set endpoint default to "both" once the upconvert bug is fixed.
	Purged TrinaryValue `json:"purged,omitempty"`

	// Newer restricts results to streams created after the specified RFC3339
	// timestamp string.
	Newer string `json:"newer,omitempty"`
	// Older restricts results to streams created before the specified RFC3339
	// timestamp string.
	Older string `json:"older,omitempty"`

	// ProtoVersion, if not "", constrains the results to those whose protobuf
	// version string matches the supplied version.
	ProtoVersion string `json:"protoVersion,omitempty"`

	// Tags is the set of tags to constrain the query with.
	//
	// A Tag entry may either be:
	// - A key/value query, in which case the results are constrained by logs
	//   whose tag includes that key/value pair.
	// - A key with an missing (nil) value, in which case the results are
	//   constraints by logs that have that tag key, regardless of its value.
	Tags []*lep.LogStreamDescriptorTag `json:"tags,omitempty"`

	// Next, if not empty, indicates that this query should continue at the point
	// where the previous query left off.
	Next string `json:"next,omitempty"`

	// MaxResults is the maximum number of query results to return.
	//
	// If MaxResults is zero, no upper bound will be indicated. However, the
	// returned result count is still be subject to internal constraints.
	MaxResults int `json:"maxResults,omitempty"`

	// State, if true, returns that the streams' full state is returned
	// instead of just its Path.
	State bool `json:"state,omitempty"`
	// Proto, if true, causes the requested state to be returned as serialized
	// protobuf data instead of deserialized JSON structures.
	Proto bool `json:"proto,omitempty"`
}

// QueryResponse is the response structure for the user Query endpoint.
type QueryResponse struct {
	// Streams is the set of streams that were identified as the result of the
	// query.
	Streams []*QueryResponseStream `json:"streams,omitempty"`

	// Next, if not empty, indicates that there are more query results available.
	// These results can be requested by repeating the Query request with the
	// same Path field and supplying this value in the Next field.
	Next string `json:"next,omitempty"`
}

// QueryResponseStream is the response structure for the user Query endpoint.
type QueryResponseStream struct {
	// Path is the log stream path.
	Path string `json:"path,omitempty"`

	// State is the log stream descriptor and state for this stream.
	//
	// It can be requested by setting the request's State field to true. If the
	// Proto field is true, the State's Descriptor field will not be included.
	State *lep.LogStreamState `json:"state,omitempty"`

	// Descriptor is the JSON-packed log stream descriptor protobuf.
	//
	// A Descriptor entry corresponds to the Path with the same index.
	//
	// If the query request's State field is set, the descriptor will be
	// populated. If the Proto field is false, Descriptor will be populated;
	// otherwise, DescriptorProto will be populated with the serialized descriptor
	// protobuf.
	Descriptor *lep.LogStreamDescriptor `json:"descriptor,omitempty"`
	// DescriptorProto is the serialized log stream Descriptor protobuf.
	DescriptorProto []byte `json:"descriptorProto,omitempty"`
}

// Query returns log stream paths that match the requested query.
func (s *Logs) Query(c context.Context, req *QueryRequest) (*QueryResponse, error) {
	c, err := s.Use(c, MethodInfoMap["Query"])
	if err != nil {
		return nil, err
	}

	// Non-admin users may not request purged results.
	canSeePurged := true
	if err := config.IsAdminUser(c); err != nil {
		canSeePurged = false

		// Non-admin user.
		if req.Purged == TrinaryYes {
			log.Fields{
				log.ErrorKey: err,
			}.Errorf(c, "Non-superuser requested to see purged logs. Denying.")
			return nil, endpoints.NewBadRequestError("non-admin user cannot request purged log streams")
		}
	}

	// Scale the maximum number of results based on the number of queries in this
	// request. If the user specified a maximum result count of zero, use the
	// default maximum.
	//
	// If this scaling results in a limit that is <1 per request, we will return
	// back a BadRequest error.
	limit := s.queryResultLimit
	if limit == 0 {
		limit = queryResultLimit
	}

	// Execute our queries in parallel.
	resp := QueryResponse{}
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
		return nil, ephelper.StripError(err)
	}
	return &resp, nil
}

type queryRunner struct {
	context.Context
	*QueryRequest

	canSeePurged bool
	limit        int
}

func (r *queryRunner) runQuery(resp *QueryResponse) error {
	if r.limit == 0 {
		return endpoints.NewBadRequestError("query limit is zero")
	}

	// Scale the maximum number of results based on the number of queries in this
	// request. If the user specified a maximum result count of zero, use the
	// default maximum.
	//
	// If this scaling results in a limit that is <1 per request, we will return
	// back a BadRequest error.
	if r.MaxResults > 0 && r.limit > r.MaxResults {
		r.limit = r.MaxResults
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
			return endpoints.NewBadRequestError("invalid `next` value")
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
			return endpoints.NewBadRequestError("invalid query `path`")
		}
	}

	if r.ContentType != "" {
		q = q.Eq("ContentType", r.ContentType)
	}

	if r.StreamType != "" {
		st, ok := streamTypeFilterMap[r.StreamType]
		if !ok {
			return endpoints.NewBadRequestError("invalid query `streamType`")
		}
		q = q.Eq("StreamType", st)
	}

	q = r.Terminated.apply(q, coordinator.AddLogStreamTerminatedFilter)
	q = r.Archived.apply(q, coordinator.AddLogStreamArchivedFilter)

	if !r.canSeePurged {
		// Force non-purged results for non-admin users.
		q = q.Eq("Purged", false)
	} else {
		q = r.Purged.apply(q, coordinator.AddLogStreamPurgedFilter)
	}

	if r.Newer != "" {
		ts, err := lep.ParseRFC3339(r.Newer)
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
				"newer":      r.Newer,
			}.Errorf(r, "Unable to parse 'Newer' field.")
			return endpoints.NewBadRequestError("invalid 'newer' value")
		}
		q = coordinator.AddNewerFilter(q, ts)
	}
	if r.Older != "" {
		ts, err := lep.ParseRFC3339(r.Older)
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
				"newer":      r.Older,
			}.Errorf(r, "Unable to parse 'Older' field.")
			return endpoints.NewBadRequestError("invalid 'newer' value")
		}
		q = coordinator.AddOlderFilter(q, ts)
	}

	if r.ProtoVersion != "" {
		q = q.Eq("ProtoVersion", r.ProtoVersion)
	}

	// Add tag constraints.
	tagsSeen := map[string]string{}
	for i, t := range r.Tags {
		tag := types.StreamTag{
			Key:   t.Key,
			Value: t.Value,
		}
		if err := tag.Validate(); err != nil {
			log.Fields{
				log.ErrorKey: err,
				"key":        tag.Key,
				"value":      tag.Value,
			}.Errorf(r, "Invalid tag constraint.")
			return endpoints.NewBadRequestError("invalid tag constraint #%d", i)
		}

		// Avoid duplicate tags.
		if v, ok := tagsSeen[tag.Key]; ok && t.Value != v {
			return endpoints.NewBadRequestError("conflicting tag query parameters #%d: [%s]", i, tag.Key)
		}
		tagsSeen[tag.Key] = tag.Value

		q = coordinator.AddLogStreamTagFilter(q, tag.Key, tag.Value)
	}

	q = q.Limit(int32(r.limit))

	cursor := ds.Cursor(nil)
	entries := make([]*coordinator.LogStream, 0, r.limit)
	err := ds.Get(r).Run(q, func(ls *coordinator.LogStream, cb ds.CursorCB) error {
		// If we hit our limit, add a cursor for the next iteration.
		entries = append(entries, ls)
		if len(entries) == r.limit {
			var err error
			cursor, err = cb()
			if err != nil {
				log.Fields{
					log.ErrorKey: err,
					"count":      len(entries),
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
		return endpoints.InternalServerError
	}

	if cursor != nil {
		resp.Next = cursor.String()
	}

	resp.Streams = make([]*QueryResponseStream, len(entries))
	for i, e := range entries {
		stream := QueryResponseStream{
			Path: string(e.Path()),
		}

		if r.State {
			stream.State = lep.LoadLogStreamState(e)

			if r.Proto {
				stream.DescriptorProto = e.Descriptor
			} else {
				stream.Descriptor, err = lep.DescriptorFromSerializedProto(e.Descriptor)
				if err != nil {
					log.Fields{
						log.ErrorKey: err,
						"prefix":     e.Prefix,
						"name":       e.Name,
						"index":      i,
					}.Errorf(r, "Failed to build descriptor from its protobuf data.")
					return endpoints.InternalServerError
				}
			}
		}
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
			}.Errorf(r, "Failed to execute query.")
			return err
		}

		resp.Streams[i] = &stream
	}
	return nil
}
