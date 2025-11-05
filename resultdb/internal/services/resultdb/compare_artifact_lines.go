// Copyright 2025 The LUCI Authors.
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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"

	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifactcontent"
	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const defaultFailurePageSize = 1000

// pageToken defines the structure of the data encoded in the page token.
type pageToken struct {
	NextByteOffset int64 `json:"b"`
	NextLineNumber int32 `json:"l"`
}

func (s *resultDBServer) CompareArtifactLines(ctx context.Context, request *pb.CompareArtifactLinesRequest) (*pb.CompareArtifactLinesResponse, error) {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	if err := artifacts.VerifyReadArtifactPermission(ctx, request.Name); err != nil {
		return nil, err
	}
	if err := validateCompareArtifactLinesRequest(request); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	startByte, startLine := int64(0), int32(0)
	if request.PageToken != "" {
		pt, err := decodePageToken(request.PageToken)
		if err != nil {
			return nil, appstatus.Errorf(codes.InvalidArgument, "page_token: %v", err)
		}
		startByte = pt.NextByteOffset
		startLine = pt.NextLineNumber
	}

	var isInvocationLevelArtifact bool
	var artifactID string

	// Determine if the failing artifact is invocation-level and get its ID.
	if pbutil.IsLegacyArtifactName(request.Name) {
		_, testID, _, artID, err := pbutil.ParseLegacyArtifactName(request.Name)
		if err != nil {
			return nil, errors.Fmt("parsing legacy artifact name %q: %w", request.Name, err)
		}
		isInvocationLevelArtifact = (testID == "")
		artifactID = artID
	} else {
		parts, err := pbutil.ParseArtifactName(request.Name)
		if err != nil {
			return nil, errors.Fmt("parsing artifact name %q: %w", request.Name, err)
		}
		isInvocationLevelArtifact = (parts.TestID == "")
		artifactID = parts.ArtifactID
	}

	var failingArt *artifacts.Artifact
	var failingStream bytestream.ByteStream_ReadClient
	passingHashes := make(map[int64]struct{})
	var passingHashesMutex sync.Mutex

	err := parallel.FanOutIn(func(c chan<- func() error) {
		c <- func() error {
			var err error
			failingArt, err = artifacts.Read(ctx, request.Name)
			if err != nil {
				return err
			}
			// TODO(b/454123742): Remove this restriction
			if failingArt.GcsUri != "" {
				return appstatus.BadRequest(fmt.Errorf("GCS artifacts not supported."))
			}
			failingStream, err = s.contentServer.ReadCASBlob(ctx, &bytestream.ReadRequest{
				ResourceName: artifactcontent.ResourceName(s.contentServer.RBECASInstanceName, failingArt.RBECASHash, failingArt.SizeBytes),
				ReadOffset:   startByte,
			})
			if err != nil {
				return errors.Fmt("creating a byte read stream for failing artifact: %w", err)
			}
			return nil
		}
		c <- func() error {
			// TODO(mwarton): Add caching for the passing line hashes.
			return parallel.FanOutIn(func(passArtifacts chan<- func() error) {
				for _, pResultName := range request.PassingResults {
					passingResultName := pResultName
					passArtifacts <- func() error {
						hashes, err := s.hashPassingArtifact(ctx, passingResultName, artifactID, isInvocationLevelArtifact)
						if err != nil {
							return err
						}
						passingHashesMutex.Lock()
						for h := range hashes {
							passingHashes[h] = struct{}{}
						}
						passingHashesMutex.Unlock()
						return nil
					}
				}
			})
		}
	})
	if err != nil {
		return nil, err
	}

	pageSize := request.GetPageSize()
	if pageSize <= 0 {
		pageSize = defaultFailurePageSize
	}
	return artifacts.ProcessFailingStream(ctx, failingStream, passingHashes, request.GetView(), pageSize, startByte, startLine)
}

func (s *resultDBServer) hashPassingArtifact(ctx context.Context, passingResultName, artifactID string, isInvocationLevelArtifact bool) (map[int64]struct{}, error) {
	var passingArtifactName string
	if isInvocationLevelArtifact {
		if pbutil.IsLegacyTestResultName(passingResultName) {
			passInvID, _, _, err := pbutil.ParseLegacyTestResultName(passingResultName)
			if err != nil {
				return nil, appstatus.BadRequest(errors.Fmt("invalid legacy passing_result_name: %s: %w", passingResultName, err))
			}
			passingArtifactName = pbutil.LegacyInvocationArtifactName(passInvID, artifactID)
		} else {
			parts, err := pbutil.ParseTestResultName(passingResultName)
			if err != nil {
				return nil, appstatus.BadRequest(errors.Fmt("invalid passing_result_name: %s: %w", passingResultName, err))
			}
			passingArtifactName = pbutil.WorkUnitArtifactName(parts.RootInvocationID, parts.WorkUnitID, artifactID)
		}
	} else {
		passingArtifactName = fmt.Sprintf("%s/artifacts/%s", passingResultName, url.PathEscape(artifactID))
	}
	art, err := artifacts.Read(ctx, passingArtifactName)
	if err != nil {
		return nil, errors.Fmt("reading passing artifact %s: %w", passingArtifactName, err)
	}
	// TODO(b/454123742): Remove this restriction
	if art.GcsUri != "" {
		return nil, appstatus.BadRequest(fmt.Errorf("GCS artifacts not supported."))
	}
	passStream, err := s.contentServer.ReadCASBlob(ctx, &bytestream.ReadRequest{
		ResourceName: artifactcontent.ResourceName(s.contentServer.RBECASInstanceName, art.RBECASHash, art.SizeBytes),
	})
	if err != nil {
		return nil, errors.Fmt("creating byte read stream for passing artifact %s: %w", passingArtifactName, err)
	}
	hashes, err := artifacts.ProcessPassingStream(ctx, passStream)
	if err != nil {
		return nil, fmt.Errorf("processing stream for passing artifact %s: %w", passingArtifactName, err)
	}
	return hashes, nil
}

func validateCompareArtifactLinesRequest(req *pb.CompareArtifactLinesRequest) error {
	// An artifact name must be either a valid legacy name OR a valid V2 name.
	if pbutil.IsLegacyArtifactName(req.Name) {
		if err := pbutil.ValidateLegacyArtifactName(req.Name); err != nil {
			return errors.Fmt("name: invalid legacy artifact name: %w", err)
		}
	} else {
		if _, err := pbutil.ParseArtifactName(req.Name); err != nil {
			return errors.Fmt("name: invalid artifact name: %w", err)
		}
	}

	if len(req.PassingResults) == 0 {
		return (errors.New("passing_result_names: must provide at least one passing result to compare against"))
	}
	for i, name := range req.PassingResults {
		var err error
		if pbutil.IsLegacyTestResultName(name) {
			err = pbutil.ValidateLegacyTestResultName(name)
		} else {
			err = pbutil.ValidateTestResultName(name)
		}
		if err != nil {
			return errors.Fmt("passing_result_names[%d]: invalid test result name", i)
		}
	}

	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Fmt("page_size: %w", err)
	}
	return nil
}

func decodePageToken(tok string) (*pageToken, error) {
	b, err := base64.StdEncoding.DecodeString(tok)
	if err != nil {
		return nil, err
	}
	pt := &pageToken{}
	if err := json.Unmarshal(b, pt); err != nil {
		return nil, err
	}
	return pt, nil
}
