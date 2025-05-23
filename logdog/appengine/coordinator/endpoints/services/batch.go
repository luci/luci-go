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

package services

import (
	"context"
	"sync"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/gcloud/gae"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
)

// The maximum, AppEngine response size, minus 1MB for overhead.
const maxResponseSize = gae.MaxResponseSize - (1024 * 1024) // 1MB

func (s *server) Batch(c context.Context, req *logdog.BatchRequest) (*logdog.BatchResponse, error) {
	// Pre-allocate our response array so we can populate it by index in
	// parallel.
	resp := logdog.BatchResponse{
		Resp: make([]*logdog.BatchResponse_Entry, 0, len(req.Req)),
	}
	if len(req.Req) == 0 {
		return &resp, nil
	}

	// Perform our batch operations in parallel. Each operation's error response
	// will be encoded in the response's "Error" parameter, so this will never
	// return an error.
	//
	// We will continue to populate the response buffer until it exceeds a size
	// constraint. If it does, we will refrain from appending it.
	var respMu sync.Mutex
	var respSize int64
	_ = parallel.WorkPool(8, func(workC chan<- func() error) {
		for i, e := range req.Req {
			workC <- func() error {
				c := logging.SetField(c, "batchIndex", i)

				r := logdog.BatchResponse_Entry{
					Index: int32(i),
				}

				s.processBatchEntry(c, e, &r)
				if err := r.GetErr(); err != nil {
					logging.Fields{
						"code":      err.GrpcCode,
						"transient": err.Transient,
					}.Errorf(c, "Failed batch entry.")
				}

				// See if this fits into our response.
				size := int64(proto.Size(&r))

				respMu.Lock()
				defer respMu.Unlock()

				if respSize+size > maxResponseSize {
					logging.Warningf(c, "Response would exceed request size (%d > %d); discarding.", respSize+size, maxResponseSize)
					return nil
				}

				resp.Resp = append(resp.Resp, &r)
				respSize += size
				return nil
			}
		}
	})
	return &resp, nil
}

func (s *server) processBatchEntry(c context.Context, e *logdog.BatchRequest_Entry, r *logdog.BatchResponse_Entry) {
	enterSingleContext := func(c context.Context, req proto.Message, fn func(context.Context) error) error {
		c, err := maybeEnterProjectNamespace(c, req)
		if err != nil {
			return err
		}
		return fn(c)
	}

	var err error
	switch req := e.Value.(type) {
	case *logdog.BatchRequest_Entry_RegisterStream:
		var resp *logdog.RegisterStreamResponse
		err = enterSingleContext(c, req.RegisterStream, func(c context.Context) (err error) {
			if resp, err = s.RegisterStream(c, req.RegisterStream); err == nil {
				r.Value = &logdog.BatchResponse_Entry_RegisterStream{RegisterStream: resp}
			}
			return
		})

	case *logdog.BatchRequest_Entry_LoadStream:
		var resp *logdog.LoadStreamResponse
		err = enterSingleContext(c, req.LoadStream, func(c context.Context) (err error) {
			if resp, err = s.LoadStream(c, req.LoadStream); err == nil {
				r.Value = &logdog.BatchResponse_Entry_LoadStream{LoadStream: resp}
			}
			return
		})

	case *logdog.BatchRequest_Entry_TerminateStream:
		err = enterSingleContext(c, req.TerminateStream, func(c context.Context) (err error) {
			_, err = s.TerminateStream(c, req.TerminateStream)
			return
		})

	case *logdog.BatchRequest_Entry_ArchiveStream:
		err = enterSingleContext(c, req.ArchiveStream, func(c context.Context) (err error) {
			_, err = s.ArchiveStream(c, req.ArchiveStream)
			return
		})

	default:
		err = status.Error(codes.InvalidArgument, "unrecognized subrequest")
	}

	// If we encountered an error, populate value with an Error.
	if err != nil {
		r.Value = &logdog.BatchResponse_Entry_Err{Err: logdog.MakeError(err)}
	}
}
