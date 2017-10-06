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
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/grpcutil"

	"github.com/golang/protobuf/proto"

	"golang.org/x/net/context"
)

func (s *server) Batch(c context.Context, req *logdog.BatchRequest) (*logdog.BatchResponse, error) {
	// Pre-allocate our response array so we can populate it by index in
	// parallel.
	resp := logdog.BatchResponse{
		Resp: make([]*logdog.BatchResponse_Entry, len(req.Req)),
	}
	if len(req.Req) == 0 {
		return &resp, nil
	}

	// Perform our batch operations in parallel. Each operation's error response
	// will be encoded in the response's "Error" parameter, so this will never
	// return an error.
	_ = parallel.FanOutIn(func(workC chan<- func() error) {
		for i, e := range req.Req {
			i, e := i, e
			workC <- func() error {
				c = logging.SetField(c, "batchIndex", i)

				var r logdog.BatchResponse_Entry
				s.processBatchEntry(c, e, &r)
				if err := r.GetErr(); err != nil {
					logging.Fields{
						"code":      err.GrpcCode,
						"transient": err.Transient,
					}.Errorf(c, "Failed batch entry.")
				}
				resp.Resp[i] = &r
				return nil
			}
		}
	})
	return &resp, nil
}

func (s *server) processBatchEntry(c context.Context, e *logdog.BatchRequest_Entry, r *logdog.BatchResponse_Entry) {
	enterSingleContext := func(c context.Context, req proto.Message, fn func(context.Context) error) error {
		logging.Debugf(c, "Batch operation: %T", req)
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
		err = grpcutil.InvalidArgument
	}

	// If we encountered an error, populate value with an Error.
	if err != nil {
		r.Value = &logdog.BatchResponse_Entry_Err{Err: logdog.MakeError(err)}
	}
}
