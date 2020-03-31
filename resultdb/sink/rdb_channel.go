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

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
)

type rdbChannel struct {
	testResultCh *dispatcher.Channel
}

func (rdbc *rdbChannel) init(ctx context.Context, gsCh *gsChannel, cfg ServerConfig) error {
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
	ch, err := dispatcher.NewChannel(ctx, rdopts, func(b *buffer.Batch) error {
		req := prepareReportTestResultsRequest(ctx, cfg.Invocation, gsCh, b)
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

func prepareReportTestResultsRequest(ctx context.Context, inv string, gsCh *gsChannel, b *buffer.Batch) *pb.BatchCreateTestResultsRequest {
	// retried batch?
	if b.Meta != nil {
		return b.Meta.(*pb.BatchCreateTestResultsRequest)
	}
	req := &pb.BatchCreateTestResultsRequest{
		Invocation: inv,
		// a random UUID
		RequestId: uuid.New().String(),
	}
	for _, d := range b.Data {
		tr := d.(*sinkpb.TestResult)
		req.Requests = append(req.Requests, &pb.CreateTestResultRequest{
			TestResult: &pb.TestResult{
				TestId:          tr.GetTestId(),
				ResultId:        tr.GetResultId(),
				Variant:         tr.GetVariant(),
				Expected:        tr.GetExpected(),
				SummaryHtml:     tr.GetSummaryHtml(),
				StartTime:       tr.GetStartTime(),
				Duration:        tr.GetDuration(),
				Tags:            tr.GetTags(),
				InputArtifacts:  sinkArtsToRpcArts(ctx, gsCh, tr.GetInputArtifacts()),
				OutputArtifacts: sinkArtsToRpcArts(ctx, gsCh, tr.GetOutputArtifacts()),
			},
		})
	}
	b.Meta = req
	return req
}

func sinkArtsToRpcArts(ctx context.Context, gsCh *gsChannel, sArts map[string]*sinkpb.Artifact) (rArts []*pb.Artifact) {
	for name, sart := range sArts {
		art := &pb.Artifact{
			Name:        name,
			Size:        -1,
			ContentType: sart.GetContentType(),
		}

		switch {
		case sart.GetFilePath() != "":
			if info, err := os.Stat(sart.GetFilePath()); err == nil {
				art.Size = info.Size()
				art.FetchUrl, art.FetchUrlExpiration = gsCh.uploadArtifact(name, sart)
			} else {
				logging.Errorf(ctx, "artifact %q: %q - %s", name, sart.GetFilePath(), err)
			}
		case sart.GetContents() != nil:
			art.Size = int64(len(sart.GetContents()))
			art.FetchUrl, art.FetchUrlExpiration = gsCh.uploadArtifact(name, sart)
		default:
			// This should never be reached. pbutil.ValidateSinkArtifact() should
			// filter out invalid artifacts.
			panic(fmt.Sprintf("%s: neither file_path nor contents were given", name))
		}
		rArts = append(rArts, art)
	}
	sort.Slice(rArts, func(i, j int) bool {
		return rArts[i].Name < rArts[j].Name
	})
	return
}
