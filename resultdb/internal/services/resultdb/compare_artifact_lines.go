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
	"sort"
	"sync"

	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifactcontent"
	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/gsutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
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
	type resolvedArtifact struct {
		name  string
		index int
	}
	var usedArtifacts []resolvedArtifact
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
			// Resolve comparison artifacts, prioritizing exact matches and then similarity.
			var comparisonArtifactNames []string
			if len(request.Artifacts) > 0 {
				comparisonArtifactNames = request.Artifacts
			} else {
				var err error
				comparisonArtifactNames, err = s.resolveComparisonArtifacts(ctx, request.PassingResults, isInvocationLevelArtifact, artifactID)
				if err != nil {
					return err
				}
			}

			if len(comparisonArtifactNames) == 0 {
				return nil
			}

			// Limit to 10 comparison artifacts.
			if len(comparisonArtifactNames) > 10 {
				comparisonArtifactNames = comparisonArtifactNames[:10]
			}

			return parallel.FanOutIn(func(c chan<- func() error) {
				for idx, artifactName := range comparisonArtifactNames {
					c <- func() error {
						// Verify ResultDB realm permission on the parent invocation/work unit
						// (needed when request.Artifacts is provided directly by the caller).
						if err := artifacts.VerifyReadArtifactPermission(ctx, artifactName); err != nil {
							code := appstatus.Code(err)
							if code == codes.PermissionDenied || code == codes.Unauthenticated || code == codes.NotFound {
								return nil
							}
							return err
						}

						// Read the artifact metadata from Spanner and stream its content from
						// RBE-CAS or GCS. This may also return NotFound if the artifact row
						// does not exist, or PermissionDenied/Unauthenticated when reading from
						// an external GCS bucket.
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
						usedArtifacts = append(usedArtifacts, resolvedArtifact{
							name:  artifactName,
							index: idx,
						})
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

	sort.Slice(usedArtifacts, func(i, j int) bool {
		return usedArtifacts[i].index < usedArtifacts[j].index
	})
	resp.Artifacts = make([]string, len(usedArtifacts))
	for i, a := range usedArtifacts {
		resp.Artifacts[i] = a.name
	}
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

// constructPassingArtifactName returns the full artifact name given a test result
// name and artifact ID. it handles both legacy and V2 test result names.
func constructPassingArtifactName(passingResultName string, isInvocationLevelArtifact bool, artifactID string) (string, error) {
	if isInvocationLevelArtifact {
		if pbutil.IsLegacyTestResultName(passingResultName) {
			passInvID, _, _, err := pbutil.ParseLegacyTestResultName(passingResultName, pbutil.QuerySideTestIDLimitCallback)
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

type artifactCandidate struct {
	name     string
	distance int
}

// resolveComparisonArtifacts concurrently resolves a set of comparison artifact names
// from the provided passing results.
//
// For each passing result, it first attempts to find an exact match for the artifactID.
// If an exact match is not found, it falls back to fuzzy matching (removing digits
// from IDs) and ranks candidates by Levenshtein distance.
//
// Returns a deduplicated and sorted list of artifact names, prioritized by match quality
// (distance) and then lexicographically.
func (s *resultDBServer) resolveComparisonArtifacts(ctx context.Context, passingResults []string, isInvocationLevelArtifact bool, artifactID string) ([]string, error) {
	var mu sync.Mutex
	var candidates []artifactCandidate
	seen := make(map[string]struct{})

	addCandidate := func(name string, distance int) {
		mu.Lock()
		defer mu.Unlock()
		if _, ok := seen[name]; !ok {
			candidates = append(candidates, artifactCandidate{name: name, distance: distance})
			seen[name] = struct{}{}
		}
	}

	err := parallel.FanOutIn(func(c chan<- func() error) {
		for _, resultName := range passingResults {
			c <- func() error {
				if pbutil.IsLegacyTestResultName(resultName) {
					exactName, err := constructPassingArtifactName(resultName, isInvocationLevelArtifact, artifactID)
					if err != nil {
						return err
					}
					if err := artifacts.VerifyReadArtifactPermission(ctx, exactName); err != nil {
						code := appstatus.Code(err)
						if code == codes.PermissionDenied || code == codes.Unauthenticated || code == codes.NotFound {
							return nil
						}
						return err
					}
					if _, err := artifacts.Read(ctx, exactName); err == nil {
						addCandidate(exactName, 0)
						return nil
					} else if appstatus.Code(err) != codes.NotFound {
						return err
					}
					invIDStr, testID, resultID, _ := pbutil.ParseLegacyTestResultName(resultName, pbutil.QuerySideTestIDLimitCallback)
					invID := invocations.ID(invIDStr)
					var parentID string
					if !isInvocationLevelArtifact {
						parentID = artifacts.ParentID(testID, resultID)
					}
					fuzzyMatches, err := artifacts.ListFuzzyMatches(ctx, invID, parentID, artifactID, 50)
					if err != nil {
						return err
					}
					for _, m := range fuzzyMatches {
						dist := artifacts.LevenshteinDistance(artifactID, m.ArtifactId)
						addCandidate(m.Name, dist)
					}
					return nil
				}

				parts, err := pbutil.ParseTestResultName(resultName)
				if err != nil {
					return appstatus.BadRequest(errors.Fmt("invalid passing_result_name: %s: %w", resultName, err))
				}
				currWUID := workunits.ID{
					RootInvocationID: rootinvocations.ID(parts.RootInvocationID),
					WorkUnitID:       parts.WorkUnitID,
				}
				var parentID string
				if !isInvocationLevelArtifact {
					parentID = artifacts.ParentID(parts.TestID, parts.ResultID)
				}

				maxSteps := 1
				if isInvocationLevelArtifact {
					maxSteps = workunits.MaxAncestorTraversalHeight + 1
				}
				for step := 0; step < maxSteps; step++ {
					if step > 0 {
						wuRow, err := workunits.Read(ctx, currWUID, workunits.ExcludeExtendedProperties)
						if err != nil || !wuRow.ParentWorkUnitID.Valid || wuRow.ParentWorkUnitID.StringVal == "" {
							break
						}
						currWUID.WorkUnitID = wuRow.ParentWorkUnitID.StringVal
					}

					var exactName string
					if isInvocationLevelArtifact {
						exactName = pbutil.WorkUnitArtifactName(string(currWUID.RootInvocationID), currWUID.WorkUnitID, artifactID)
					} else {
						exactName = fmt.Sprintf("%s/artifacts/%s", resultName, url.PathEscape(artifactID))
					}

					if err := artifacts.VerifyReadArtifactPermission(ctx, exactName); err != nil {
						code := appstatus.Code(err)
						if code == codes.PermissionDenied || code == codes.Unauthenticated || code == codes.NotFound {
							continue
						}
						return err
					}

					if _, err := artifacts.Read(ctx, exactName); err == nil {
						addCandidate(exactName, 0)
						return nil
					} else if appstatus.Code(err) != codes.NotFound {
						return err
					}

					fuzzyMatches, err := artifacts.ListFuzzyMatches(ctx, currWUID.LegacyInvocationID(), parentID, artifactID, 50)
					if err != nil {
						return err
					}
					if len(fuzzyMatches) > 0 {
						for _, m := range fuzzyMatches {
							dist := artifacts.LevenshteinDistance(artifactID, m.ArtifactId)
							addCandidate(m.Name, dist)
						}
						return nil
					}
				}
				return nil
			}
		}
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].distance != candidates[j].distance {
			return candidates[i].distance < candidates[j].distance
		}
		return candidates[i].name < candidates[j].name
	})

	names := make([]string, len(candidates))
	for i, c := range candidates {
		names[i] = c.name
	}
	return names, nil
}
