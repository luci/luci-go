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
	"io"

	"google.golang.org/genproto/googleapis/bytestream"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifactcontent"
	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// The maximum number of bytes we return is 10mb
const maxBytes int = 10_000_000

func (s *resultDBServer) ListArtifactLines(ctx context.Context, in *pb.ListArtifactLinesRequest) (*pb.ListArtifactLinesResponse, error) {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	if err := artifacts.VerifyReadArtifactPermission(ctx, in.Parent); err != nil {
		return nil, err
	}

	if err := validateListArtifactLinesRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	invID, _, _, artifactID, err := pbutil.ParseArtifactName(in.Parent)

	if err != nil {
		return nil, errors.Annotate(err, "parse artifact name").Err()
	}

	invocation, err := invocations.Read(ctx, invocations.ID(invID))

	if err != nil {
		return nil, errors.Annotate(err, "reading invocation").Err()
	}

	art, err := artifacts.Read(ctx, in.Parent)
	if err != nil {
		return nil, err
	}

	content, err := s.readArtifactData(ctx, art)

	if err != nil {
		return nil, errors.Annotate(err, "read artifact").Err()
	}

	year := invocation.CreateTime.AsTime().Year()

	lines, err := artifacts.ToLogLines(artifactID, art.ContentType, content, year, int(in.PageSize), maxBytes)

	if err != nil {
		return nil, errors.Annotate(err, "process lines").Err()
	}

	resp := &pb.ListArtifactLinesResponse{
		Lines: lines,
	}

	return resp, nil
}

func validateListArtifactLinesRequest(req *pb.ListArtifactLinesRequest) error {
	if err := pbutil.ValidateArtifactName(req.Parent); err != nil {
		return appstatus.BadRequest(errors.Reason("parent: invalid artifact name").Err())
	}

	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return appstatus.BadRequest(errors.Annotate(err, "page_size").Err())
	}

	return nil
}

func (s *resultDBServer) readArtifactData(ctx context.Context, art *artifacts.Artifact) ([]byte, error) {
	stream, err := s.contentServer.ReadCASBlob(ctx, &bytestream.ReadRequest{
		ResourceName: artifactcontent.ResourceName(s.contentServer.RBECASInstanceName, art.RBECASHash, art.SizeBytes),
	})

	if err != nil {
		return nil, errors.Annotate(err, "creating a byte read stream").Err()
	}

	allData := make([]byte, 0, art.SizeBytes)
	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break // End of stream reached
		}
		if err != nil {
			return nil, errors.Annotate(err, "reading bytes for artifact").Err()
		}

		allData = append(allData, response.Data...)
	}
	return allData, nil
}
