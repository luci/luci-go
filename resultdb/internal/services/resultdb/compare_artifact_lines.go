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
	"io"
	"net/url"
	"sync"

	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifactcontent"
	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/gsutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/workunits"
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
	var failingReader io.ReadCloser
	comparisonHashes := make(map[int64]struct{})
	var usedArtifacts []string
	var mu sync.Mutex

	gsClients := make(map[string]gsutil.Client)
	defer func() {
		for _, c := range gsClients {
			c.Close()
		}
	}()

	getGSClient := func(ctx context.Context, project string) (gsutil.Client, error) {
		mu.Lock()
		defer mu.Unlock()
		if c, ok := gsClients[project]; ok {
			return c, nil
		}
		c, err := gsutil.NewStorageClient(ctx, project)
		if err != nil {
			return nil, err
		}
		gsClients[project] = c
		return c, nil
	}

	err := parallel.FanOutIn(func(tasks chan<- func() error) {
		tasks <- func() error {
			var err error
			failingArt, err = artifacts.Read(ctx, request.Name)
			if err != nil {
				return err
			}

			failingReader, err = s.openArtifactReader(ctx, failingArt, startByte, getGSClient)
			return err
		}
		tasks <- func() error {
			// TODO(mwarton): Add caching for the comparison line hashes.
			return parallel.FanOutIn(func(comparisonTasks chan<- func() error) {
				var comparisonArtifactNames []string
				if len(request.Artifacts) > 0 {
					comparisonArtifactNames = append(comparisonArtifactNames, request.Artifacts...)
				} else {
					for _, resultName := range request.PassingResults {
						comparisonArtifactName, err := constructPassingArtifactName(resultName, isInvocationLevelArtifact, artifactID)
						if err != nil {
							logging.Warningf(ctx, "Failed to construct passing artifact name for %q: %v", resultName, err)
							continue
						}
						comparisonArtifactNames = append(comparisonArtifactNames, comparisonArtifactName)
					}
				}

				for _, artName := range comparisonArtifactNames {
					artifactName := artName
					comparisonTasks <- func() error {
						if err := artifacts.VerifyReadArtifactPermission(ctx, artifactName); err != nil {
							code := appstatus.Code(err)
							if code == codes.PermissionDenied || code == codes.Unauthenticated || code == codes.NotFound {
								return nil
							}
							return err
						}

						hashes, err := s.hashArtifact(ctx, artifactName, getGSClient)
						if err != nil {
							code := appstatus.Code(err)
							if code == codes.PermissionDenied || code == codes.Unauthenticated || code == codes.NotFound {
								return nil
							}
							return err
						}
						mu.Lock()
						for h := range hashes {
							comparisonHashes[h] = struct{}{}
						}
						usedArtifacts = append(usedArtifacts, artifactName)
						mu.Unlock()
						return nil
					}
				}
			})
		}
	})
	if err != nil {
		return nil, err
	}
	defer failingReader.Close()

	pageSize := request.GetPageSize()
	if pageSize <= 0 {
		pageSize = defaultFailurePageSize
	}

	resp, err := artifacts.ProcessFailingReader(ctx, failingReader, comparisonHashes, request.GetView(), pageSize, startByte, startLine)
	if err != nil {
		return nil, err
	}

	resp.Artifacts = usedArtifacts
	return resp, nil
}

func (s *resultDBServer) hashArtifact(ctx context.Context, artifactName string, getGSClient func(context.Context, string) (gsutil.Client, error)) (map[int64]struct{}, error) {
	art, err := artifacts.Read(ctx, artifactName)
	if err != nil {
		return nil, errors.Fmt("reading comparison artifact %s: %w", artifactName, err)
	}

	reader, err := s.openArtifactReader(ctx, art, 0, getGSClient)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	hashes, err := artifacts.ProcessComparisonReader(ctx, reader)
	if err != nil {
		return nil, fmt.Errorf("processing reader for comparison artifact %s: %w", artifactName, err)
	}
	return hashes, nil
}

func (s *resultDBServer) openArtifactReader(ctx context.Context, art *artifacts.Artifact, offset int64, getGSClient func(context.Context, string) (gsutil.Client, error)) (io.ReadCloser, error) {
	project, err := s.projectForArtifact(ctx, art.Artifact.Name)
	if err != nil {
		return nil, err
	}

	if art.GcsUri != "" {
		bucket, object := gsutil.Split(art.GcsUri)
		gsClient, err := getGSClient(ctx, project)
		if err != nil {
			return nil, err
		}
		reader, err := gsClient.NewReader(ctx, bucket, object, offset)
		if err != nil {
			return nil, errors.Fmt("creating GCS reader for artifact %s: %w", art.Artifact.Name, err)
		}
		return reader, nil
	}

	comparisonStream, err := s.contentServer.ReadCASBlob(ctx, &bytestream.ReadRequest{
		ResourceName: artifactcontent.ResourceName(s.contentServer.RBECASInstanceName, art.RBECASHash, art.SizeBytes),
		ReadOffset:   offset,
	})
	if err != nil {
		return nil, errors.Fmt("creating byte read stream for artifact %s: %w", art.Artifact.Name, err)
	}
	return &bytestreamReader{stream: comparisonStream}, nil
}

func (s *resultDBServer) projectForArtifact(ctx context.Context, name string) (string, error) {
	var realm string
	var err error
	if pbutil.IsLegacyArtifactName(name) {
		invIDStr, _, _, _ := artifacts.MustParseLegacyName(name)
		realm, err = invocations.ReadRealm(ctx, invocations.ID(invIDStr))
	} else {
		wuID, _, _, _ := artifacts.MustParseName(name)
		realm, err = workunits.ReadRealm(ctx, wuID)
	}
	if err != nil {
		return "", err
	}
	project, _ := realms.Split(realm)
	return project, nil
}

// implements io.Reader for a bytestream.ByteStream_ReadClient.
type bytestreamReader struct {
	stream bytestream.ByteStream_ReadClient
	buf    []byte
}

func (r *bytestreamReader) Read(p []byte) (n int, err error) {
	if len(r.buf) == 0 {
		resp, err := r.stream.Recv()
		if err != nil {
			return 0, err
		}
		r.buf = resp.Data
	}
	n = copy(p, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}

func (r *bytestreamReader) Close() error {
	return nil
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

	if len(req.Artifacts) > 0 {
		if len(req.PassingResults) > 0 {
			return (errors.New("only one of passing_results and artifacts may be set"))
		}
		for i, name := range req.Artifacts {
			if pbutil.IsLegacyArtifactName(name) {
				if err := pbutil.ValidateLegacyArtifactName(name); err != nil {
					return errors.Fmt("artifacts[%d]: invalid legacy artifact name: %w", i, err)
				}
			} else {
				if _, err := pbutil.ParseArtifactName(name); err != nil {
					return errors.Fmt("artifacts[%d]: invalid artifact name: %w", i, err)
				}
			}
		}
	} else {
		if len(req.PassingResults) == 0 {
			return (errors.New("must provide at least one passing result OR artifact to compare against"))
		}
		for i, name := range req.PassingResults {
			var err error
			if pbutil.IsLegacyTestResultName(name) {
				err = pbutil.ValidateLegacyTestResultName(name)
			} else {
				err = pbutil.ValidateTestResultName(name)
			}
			if err != nil {
				return errors.Fmt("passing_results[%d]: invalid test result name", i)
			}
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

func constructPassingArtifactName(passingResultName string, isInvocationLevelArtifact bool, artifactID string) (string, error) {
	if isInvocationLevelArtifact {
		if pbutil.IsLegacyTestResultName(passingResultName) {
			passInvID, _, _, err := pbutil.ParseLegacyTestResultName(passingResultName)
			if err != nil {
				return "", appstatus.BadRequest(errors.Fmt("invalid legacy passing_result_name: %s: %w", passingResultName, err))
			}
			return pbutil.LegacyInvocationArtifactName(passInvID, artifactID), nil
		}
		parts, err := pbutil.ParseTestResultName(passingResultName)
		if err != nil {
			return "", appstatus.BadRequest(errors.Fmt("invalid passing_result_name: %s: %w", passingResultName, err))
		}
		return pbutil.WorkUnitArtifactName(parts.RootInvocationID, parts.WorkUnitID, artifactID), nil
	}
	return fmt.Sprintf("%s/artifacts/%s", passingResultName, url.PathEscape(artifactID)), nil
}
