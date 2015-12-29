// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logs

import (
	"time"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/golang/protobuf/proto"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	lep "github.com/luci/luci-go/appengine/logdog/coordinator/endpoints"
	"github.com/luci/luci-go/common/logdog/protocol"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry"
	"github.com/luci/luci-go/server/logdog/storage"
	"golang.org/x/net/context"
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

// GetRequest is the request structure for the user Get endpoint.
//
// If the requested log stream exists, a valid GetRequest will succeed
// regardless of whether the requested log range was available.
//
// Note that this endpoint may return fewer logs than requested due to either
// availability or internal constraints.
type GetRequest struct {
	// Path is the path of the log stream to get.
	//
	// This can either be a LogDog stream path or the SHA256 hash of a LogDog
	// stream path.
	//
	// Some utilities may find passing around a full LogDog path to be cumbersome
	// due to its length. They can opt to pass around the hash instead and
	// retrieve logs using it.
	Path string `endpoints:"required"`

	// State, if true, requests that the log stream's state is returned.
	State bool `json:"state,omitempty"`

	// Index is the initial log stream index to retrieve.
	Index int64 `json:"index,omitempty,string"`

	// Bytes is the maximum number of bytes to return. If non-zero, it is applied
	// as a constraint to limit the number of logs that are returned.
	//
	// Note that if the first log record exceeds this value, it will be returned
	// regardless of the Bytes constraint.
	Bytes int `json:"bytes,omitempty"`
	// Count is the maximum number of log records to request.
	//
	// If this value is zero, no count constraint will be applied. If this value
	// is less than zero, no log entries will be returned.
	Count int `json:"count,omitempty"`

	// Tail, if true, requests that the returned logs start from the end of the
	// log stream rather than the beginning. The logs will be returned in reverse
	// order, from highest to lowest index.
	//
	// The end of the stream is defined as the highest index in the contiguous
	// log entry index space starting from Index.
	Tail bool `json:"tail,omitempty"`

	// NonContiguous, if true, allows the range request to return non-contiguous
	// records.
	//
	// A contiguous request (default) will iterate forwards from the supplied
	// Index and stop if either the end of stream is encountered or there is a
	// missing stream index. A NonContiguous request will remove the latter
	// condition.
	//
	// For example, say the log stream consists of:
	// [3, 4, 6, 7]
	//
	// A contiguous request with Index 3 will return: [3, 4], stopping because
	// 5 is missing. A non-contiguous request will return [3, 4, 6, 7].
	//
	// Since tail logs are, by definition, contiguous, this flag is ignored when
	// requesting tail logs.
	NonContiguous bool `json:"noncontiguous,omitempty"`

	// Proto, if true, causes the requested state and log data to be returned
	// as serialized protobuf data instead of deserialized JSON structures.
	Proto bool `json:"proto,omitempty"`
	// Newlines, if true, causes the lines returned for text streams to include
	// their newline delimiters. If false, text stream lines will be returned
	// without delimiters.
	//
	// This is only applicable when the requested stream is a text stream and
	// Proto is false.
	Newlines bool `json:"newlines,omitempty"`
}

// GetResponse is the response structure for the user Get endpoint.
type GetResponse struct {
	// State is the log stream descriptor and state for this stream.
	//
	// It can be requested by setting the request's State field to true. If the
	// Proto field is true, the State's Descriptor field will not be included.
	State *lep.LogStreamState `json:"state,omitempty"`

	// Descriptor is the expanded LogStreamDescriptor protobuf. It is intended
	// for JSON consumption.
	//
	// If the GetRequest's Proto field is false, this will be populated;
	// otherwise, the serialized protobuf will be written to the DescriptorProto
	// field.
	Descriptor *lep.LogStreamDescriptor `json:"descriptor,omitempty"`
	// DescriptorProto is the serialized log stream descriptor protobuf value.
	//
	// It can be requested by setting the request's State and Proto fields to
	// true.
	DescriptorProto []byte `json:"descriptorProto,omitempty"`

	// Logs is the set of logs retireved logs.
	Logs []*GetLogEntry `json:"logs,omitempty"`
}

// GetLogEntry is the data for a single returned LogEntry.
type GetLogEntry struct {
	// Entry is the requested range of log entries.
	//
	// If the range was not available, Logs will be empty.
	Entry *LogEntry `json:"entry,omitempty"`

	// Proto is the log's serialized protobuf data.
	//
	// If the request's Proto field is true, this will be populated instead of the
	// Logs field.
	Proto []byte `json:"proto,omitempty"`
}

// Get returns state and log data for a single log stream.
func (s *Logs) Get(c context.Context, req *GetRequest) (*GetResponse, error) {
	c, err := s.Use(c, MethodInfoMap["Get"])
	if err != nil {
		return nil, err
	}

	// Fetch the log stream state for this log stream.
	ls, err := coordinator.NewLogStream(req.Path)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"path":       req.Path,
		}.Errorf(c, "Invalid path supplied.")
		return nil, endpoints.NewBadRequestError("invalid path value")
	}

	// If nothing was requested, return nothing.
	if !req.State && req.Count < 0 {
		return nil, nil
	}

	// If this log entry is Purged and we're not admin, pretend it doesn't exist.
	err = ds.Get(c).Get(ls)
	if err == nil && ls.Purged {
		if authErr := config.IsAdminUser(c); authErr != nil {
			log.Fields{
				log.ErrorKey: authErr,
			}.Warningf(c, "Non-superuser requested purged log.")
			err = ds.ErrNoSuchEntity
		}
	}
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"path":       req.Path,
		}.Errorf(c, "Failed to look up log stream.")

		if err != ds.ErrNoSuchEntity {
			return nil, endpoints.InternalServerError
		}
		return nil, endpoints.NewNotFoundError("path not found")
	}
	path := ls.Path()

	// Construct response...
	resp := GetResponse{}

	if req.State {
		resp.State = lep.LoadLogStreamState(ls)
		if req.Proto {
			resp.DescriptorProto = ls.Descriptor
		} else {
			resp.Descriptor, err = lep.DescriptorFromSerializedProto(ls.Descriptor)
			if err != nil {
				log.Fields{
					log.ErrorKey: err,
				}.Errorf(c, "Failed to deserialize descriptor protobuf.")
				return nil, endpoints.InternalServerError
			}
		}
	}

	// Retrieve requested logs from storage, if requested.
	if req.Count >= 0 {
		st, err := s.getStorage(c)
		if err != nil {
			return nil, err
		}
		defer st.Close()

		logs, err := s.getLogs(c, st, req, ls)
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
				"path":       path,
			}.Errorf(c, "Failed to get logs.")
			return nil, endpoints.InternalServerError
		}

		// Stuff the logs into our response.
		resp.Logs = make([]*GetLogEntry, len(logs))
		for idx, ld := range logs {
			gle := GetLogEntry{}
			if req.Proto {
				gle.Proto = ld
			} else {
				// Deserialize the log entry, then convert it to output value.
				le := protocol.LogEntry{}
				if err := proto.Unmarshal(ld, &le); err != nil {
					log.Fields{
						log.ErrorKey: err,
						"logIndex":   idx,
					}.Errorf(c, "Failed to unmarshal LogEntry from log data.")
					return nil, endpoints.InternalServerError
				}
				gle.Entry = logEntryFromProto(&le, ls.Timestamp, req.Newlines)
			}

			resp.Logs[idx] = &gle
		}
	}

	return &resp, nil
}

func (*Logs) getLogs(c context.Context, st storage.Storage, req *GetRequest, ls *coordinator.LogStream) ([][]byte, error) {
	c = log.SetFields(c, log.Fields{
		"index":         req.Index,
		"count":         req.Count,
		"bytes":         req.Bytes,
		"noncontiguous": req.NonContiguous,
	})

	byteLimit := req.Bytes
	if byteLimit <= 0 || byteLimit > getBytesLimit {
		byteLimit = getBytesLimit
	}

	// Allocate result logs array.
	asz := getInitialArraySize
	if req.Count > 0 && req.Count < asz {
		asz = req.Count
	}
	logs := make([][]byte, 0, asz)

	meth := st.Get
	if req.Tail {
		meth = st.Tail
	}

	sreq := storage.GetRequest{
		Path:  ls.Path(),
		Index: types.MessageIndex(req.Index),
		Limit: req.Count,
	}

	count := 0
	err := retry.Retry(c, retry.TransientOnly(retry.Default()), func() error {
		// Issue the Get request. This may return a transient error, in which case
		// we will retry.
		return meth(&sreq, func(idx types.MessageIndex, ld []byte) bool {
			if count > 0 && byteLimit-len(ld) < 0 {
				// Not the first log, and we've exceeded our byte limit.
				return false
			}
			byteLimit -= len(ld)

			if !req.Tail {
				// Head request. Enforce contiguous.
				if !(req.NonContiguous || idx == sreq.Index) {
					return false
				}
				logs = append(logs, ld)
				sreq.Index = idx + 1
			} else {
				// Tail request.
				logs = append(logs, ld)
				sreq.Index = idx - 1
			}

			count++
			return !(req.Count > 0 && count >= req.Count)
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
