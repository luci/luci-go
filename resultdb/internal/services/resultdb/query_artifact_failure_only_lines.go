// Copyright 2024 The LUCI Authors.
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

package resultdb

import (
	"context"
	"fmt"
	"io"

	"google.golang.org/genproto/googleapis/bytestream"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifactcontent"
	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// The number of passing artifacts to sample to identify failure only log lines.
const numPassesToCompare = 10

func (s *resultDBServer) QueryArtifactFailureOnlyLines(ctx context.Context, request *pb.QueryArtifactFailureOnlyLinesRequest) (*pb.QueryArtifactFailureOnlyLinesResponse, error) {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	if err := artifacts.VerifyReadArtifactPermission(ctx, request.Parent); err != nil {
		return nil, err
	}

	if err := validateQueryArtifactFailureOnlyLinesRequest(request); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	invID, testID, _, artifactID, err := pbutil.ParseArtifactName(request.Parent)
	if err != nil {
		return nil, errors.Annotate(err, "parse artifact name").Err()
	}

	if testID == "" {
		return nil, appstatus.BadRequest(fmt.Errorf("only test artifacts are supported, %s is an invocation level artifact", request.Parent))
	}

	invocation, err := invocations.Read(ctx, invocations.ID(invID))

	if err != nil {
		return nil, errors.Annotate(err, "reading invocation").Err()
	}

	var art *artifacts.Artifact
	var contentChunk []byte
	var stream bytestream.ByteStream_ReadClient
	eof := false
	passingHashes := map[int64]struct{}{}

	err = parallel.FanOutIn(func(c chan<- func() error) {
		c <- func() error {
			var err error
			art, err = artifacts.Read(ctx, request.Parent)
			if err != nil {
				return err
			}

			stream, err = s.contentServer.ReadCASBlob(ctx, &bytestream.ReadRequest{
				ResourceName: artifactcontent.ResourceName(s.contentServer.RBECASInstanceName, art.RBECASHash, art.SizeBytes),
			})

			if err != nil {
				return errors.Annotate(err, "creating a byte read stream").Err()
			}

			// Read the first chunk in parallel with BigQuery.
			response, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				eof = true
				return nil
			}
			if err != nil {
				return errors.Annotate(err, "reading bytes for artifact").Err()
			}
			contentChunk = response.Data
			return nil
		}
		c <- func() error {
			// TODO: get the artifact date and test variant for more accurate comparisons.
			passingHashes, err = artifacts.FetchPassingHashes(ctx, s.bqClient, invocation.Realm, testID, artifactID, numPassesToCompare)
			if err != nil {
				return errors.Annotate(err, "get passing hashes").Err()
			}
			return err
		}
	})
	if err != nil {
		return nil, err
	}

	// TODO(mwarton): Need to handle the edge cases of a line spanning two
	// chunks. Leaving it for a follow up CL as it should rarely be a problem
	// because most logs are small.
	ranges := []*pb.QueryArtifactFailureOnlyLinesResponse_LineRange{}
	for !eof {
		chunkRanges, err := artifacts.ToFailureOnlyLineRanges(artifactID, art.ContentType, contentChunk, passingHashes, request.IncludeContent)
		if err != nil {
			return nil, errors.Annotate(err, "process lines").Err()
		}
		// If the first range in the new chunk is a continuation of the last range
		// in the previous chunk, merge them together.
		if len(ranges) > 0 && len(chunkRanges) > 0 && ranges[len(ranges)-1].End == chunkRanges[0].Start {
			ranges[len(ranges)-1].End = chunkRanges[0].End
			ranges[len(ranges)-1].Lines = append(ranges[len(ranges)-1].Lines, chunkRanges[0].Lines...)
			ranges = append(ranges, chunkRanges[1:]...)
		} else {
			ranges = append(ranges, chunkRanges...)
		}

		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			eof = true
			break
		}
		if err != nil {
			return nil, errors.Annotate(err, "reading bytes for artifact").Err()
		}
		contentChunk = response.Data
	}

	resp := &pb.QueryArtifactFailureOnlyLinesResponse{
		FailureOnlyLineRanges: ranges,
	}

	// TODO: implement paging.  For now we will just return everything.
	return resp, nil
}

func validateQueryArtifactFailureOnlyLinesRequest(req *pb.QueryArtifactFailureOnlyLinesRequest) error {
	if err := pbutil.ValidateArtifactName(req.Parent); err != nil {
		return appstatus.BadRequest(errors.Reason("parent: invalid artifact name").Err())
	}

	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return appstatus.BadRequest(errors.Annotate(err, "page_size").Err())
	}

	return nil
}
