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

package resultdb

import (
	"context"
	"time"

	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/gsutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func validateGetArtifactRequest(req *pb.GetArtifactRequest) error {
	if err := pbutil.ValidateLegacyArtifactName(req.Name); err != nil {
		return errors.Fmt("name: %w", err)
	}

	return nil
}

// GetArtifact implements pb.ResultDBServer.
func (s *resultDBServer) GetArtifact(ctx context.Context, in *pb.GetArtifactRequest) (*pb.Artifact, error) {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	if err := artifacts.VerifyReadArtifactPermission(ctx, in.Name); err != nil {
		return nil, err
	}

	if err := validateGetArtifactRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	artData, err := artifacts.Read(ctx, in.Name)
	if err != nil {
		return nil, err
	}

	art := artData.Artifact

	invIDStr, _, _, _ := artifacts.MustParseLegacyName(in.Name)
	realm, err := invocations.ReadRealm(ctx, invocations.ID(invIDStr))
	if err != nil {
		return nil, err
	}
	project, _ := realms.Split(realm)
	workUnitIDToProject := map[workunits.ID]string{}
	invocationIDToProject := map[invocations.ID]string{
		invocations.ID(invIDStr): project,
	}
	if err := s.populateFetchURLs(ctx, workUnitIDToProject, invocationIDToProject, art); err != nil {
		return nil, err
	}

	return art, nil
}

// populateFetchURLs populates FetchUrl and FetchUrlExpiration fields
// of the artifacts.
// invocationIDToProject contains a mapping between invocation ids and their LUCI project, it should contain all immediate parent invocations of legacy artifacts.
// workUnitIDToProject contains a mapping between work unit ids and their LUCI project, it should contain all immediate parent work units of artifacts.
//
// Must be called from within some gRPC request handler.
func (s *resultDBServer) populateFetchURLs(ctx context.Context, workUnitIDToProject map[workunits.ID]string, invocationIDToProject map[invocations.ID]string, arts ...*pb.Artifact) error {
	// Extract Host header (may be empty) from the request to use it as a basis
	// for generating artifact URLs.
	requestHost := ""
	md, _ := metadata.FromIncomingContext(ctx)
	if val := md.Get("host"); len(val) > 0 {
		requestHost = val[0]
	}

	// Mapping of LUCI project to google storage Client.
	// Each client will sent request authenticated using the corresponding project-scoped service account.
	gsClients := map[string]gsutil.Client{}
	now := clock.Now(ctx).UTC()

	for _, a := range arts {
		var project string
		if pbutil.IsLegacyArtifactName(a.Name) {
			invIDStr, _, _, _ := artifacts.MustParseLegacyName(a.Name)
			var ok bool
			project, ok = invocationIDToProject[invocations.ID(invIDStr)]
			if !ok {
				panic("invocation for artifact doesn't exist in the invocationIDToProject map")
			}
		} else {
			wuID, _, _, _ := artifacts.MustParseName(a.Name)
			var ok bool
			project, ok = workUnitIDToProject[wuID]
			if !ok {
				panic("work unit for artifact doesn't exist in the workUnitIDToProject map")
			}
		}

		if a.GcsUri != "" {
			// ResultDB allows including invocation from a different LUCI project to a parent invocation.
			// We should use the LUCI project of the immediate parent of the artifact to generate the signed URL.
			if _, ok := gsClients[project]; !ok {
				c, err := gsutil.NewStorageClient(ctx, project)
				if err != nil {
					return errors.Fmt("new storage client for project %s: %w", project, err)
				}
				defer c.Close()
				gsClients[project] = c
			}

			exp := now.Add(7 * 24 * time.Hour)
			bucket, object := gsutil.Split(a.GcsUri)
			gsClient := gsClients[project]
			// The QPS of GenerateSignedURL is limited by the underlying
			// projects.serviceAccounts.signBlob call, which has a quota of 60,000
			// requests per minute. Exceeding this limit will result in a quota error.
			url, err := gsClient.GenerateSignedURL(ctx, bucket, object, exp)
			if err != nil {
				return err
			}

			a.FetchUrl = url
			a.FetchUrlExpiration = pbutil.MustTimestampProto(exp)
		} else if a.GetRbeUri() != "" {
			// Specify the project name to the token to ensure that the correct
			// project-scoped service account is used to access the RBE
			// artifact.
			embedded := map[string]string{
				"project": project,
			}
			url, exp, err := s.contentServer.GenerateSignedURL(ctx, requestHost, a.Name, embedded)
			if err != nil {
				return err
			}

			a.FetchUrl = url
			a.FetchUrlExpiration = pbutil.MustTimestampProto(exp)
		} else {
			// Skip the project name so that the ResultDB service account is
			// used to access the artifact.
			url, exp, err := s.contentServer.GenerateSignedURL(ctx, requestHost, a.Name, nil)
			if err != nil {
				return err
			}

			a.FetchUrl = url
			a.FetchUrlExpiration = pbutil.MustTimestampProto(exp)
		}
	}
	return nil
}
