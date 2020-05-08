// Copyright 2020 The LUCI Authors.
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

package sink

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"

	"go.chromium.org/luci/resultdb/internal/services/recorder"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
)

type rdbChannel struct {
	testResultCh *dispatcher.Channel
}

func (rdbc *rdbChannel) init(ctx context.Context, cfg ServerConfig) error {
	// install a dispatcher channel for pb.TestResult
	rdopts := &dispatcher.Options{
		QPSLimit: rate.NewLimiter(1, 1),
		Buffer: buffer.Options{
			BatchSize:     400,
			MaxLeases:     4,
			BatchDuration: 10 * time.Second,
			FullBehavior:  &buffer.BlockNewItems{MaxItems: 2000},
		},
	}

	ctx = metadata.AppendToOutgoingContext(ctx, recorder.UpdateTokenMetadataKey, cfg.UpdateToken)
	ch, err := dispatcher.NewChannel(ctx, rdopts, func(b *buffer.Batch) error {
		req := prepareReportTestResultsRequest(ctx, &cfg, b)
		_, err := cfg.Recorder.BatchCreateTestResults(ctx, req)
		return err
	})
	if err != nil {
		return err
	}
	rdbc.testResultCh = &ch
	return nil
}

func (rdbc *rdbChannel) closeAndDrain(ctx context.Context) {
	rdbc.testResultCh.CloseAndDrain(ctx)
}

func (rdbc *rdbChannel) reportTestResults(trs []*sinkpb.TestResult) {
	for _, tr := range trs {
		rdbc.testResultCh.C <- tr
	}
}

func prepareReportTestResultsRequest(ctx context.Context, cfg *ServerConfig, b *buffer.Batch) *pb.BatchCreateTestResultsRequest {
	// retried batch?
	if b.Meta != nil {
		return b.Meta.(*pb.BatchCreateTestResultsRequest)
	}
	req := &pb.BatchCreateTestResultsRequest{
		Invocation: cfg.Invocation,
		// a random UUID
		RequestId: uuid.New().String(),
	}
	for _, d := range b.Data {
		tr := d.(*sinkpb.TestResult)
		req.Requests = append(req.Requests, &pb.CreateTestResultRequest{
			TestResult: &pb.TestResult{
				TestId:      tr.GetTestId(),
				ResultId:    tr.GetResultId(),
				Variant:     tr.GetVariant(),
				Expected:    tr.GetExpected(),
				SummaryHtml: tr.GetSummaryHtml(),
				StartTime:   tr.GetStartTime(),
				Duration:    tr.GetDuration(),
				Tags:        tr.GetTags(),
			},
		})
	}
	b.Meta = req
	return req
}

func sinkArtsToRPCArts(ctx context.Context, sArts map[string]*sinkpb.Artifact) (rArts []*pb.Artifact) {
	for name, sart := range sArts {
		var size int64 = -1
		switch {
		case sart.GetFilePath() != "":
			if info, err := os.Stat(sart.GetFilePath()); err == nil {
				size = info.Size()
			} else {
				logging.Errorf(ctx, "artifact %q: %q - %s", name, sart.GetFilePath(), err)
			}
		case sart.GetContents() != nil:
			size = int64(len(sart.GetContents()))
		default:
			// This should never be reached. pbutil.ValidateSinkArtifact() should
			// filter out invalid artifacts.
			panic(fmt.Sprintf("%s: neither file_path nor contents were given", name))
		}

		rArts = append(rArts, &pb.Artifact{
			Name: name,
			// TODO(ddoman): set fetch_url and fetch_url_expiration
			ContentType: sart.GetContentType(),
			SizeBytes:   size,
		})
	}
	sort.Slice(rArts, func(i, j int) bool {
		return rArts[i].Name < rArts[j].Name
	})
	return
}
