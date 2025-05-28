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

	"cloud.google.com/go/storage"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/gsutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func validateGetArtifactRequest(req *pb.GetArtifactRequest) error {
	if err := pbutil.ValidateArtifactName(req.Name); err != nil {
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

	invIDStr, _, _, _, _ := pbutil.ParseArtifactName(in.Name)
	realm, err := invocations.ReadRealm(ctx, invocations.ID(invIDStr))
	if err != nil {
		return nil, err
	}

	if err := s.populateFetchURLs(ctx, []string{realm}, art); err != nil {
		return nil, err
	}

	return art, nil
}

// populateFetchURLs populates FetchUrl and FetchUrlExpiration fields
// of the artifacts. Uses queriedRealms for GCS Artifacts ACL checking.
//
// Must be called from within some gRPC request handler.
func (s *resultDBServer) populateFetchURLs(ctx context.Context, queriedRealms []string, artifacts ...*pb.Artifact) error {
	// Extract Host header (may be empty) from the request to use it as a basis
	// for generating artifact URLs.
	requestHost := ""
	md, _ := metadata.FromIncomingContext(ctx)
	if val := md.Get("host"); len(val) > 0 {
		requestHost = val[0]
	}

	// Client to fetch from Google Storage
	var gsClient *storage.Client
	now := clock.Now(ctx).UTC()

	for _, a := range artifacts {
		if a.GcsUri != "" {
			if gsClient == nil {
				client, err := storage.NewClient(ctx)
				if err != nil {
					return err
				}
				gsClient = client
			}

			exp := now.Add(7 * 24 * time.Hour)
			var opts *storage.SignedURLOptions
			ctxOpts := ctx.Value(gsutil.Key("signedURLOpts"))
			if ctxOpts != nil {
				opts = ctxOpts.(*storage.SignedURLOptions)
			}
			bucket, object := gsutil.Split(a.GcsUri)
			url, err := gsutil.GenerateSignedURL(ctx, gsClient, bucket, object, exp, opts)

			if err != nil {
				return err
			}

			a.FetchUrl = url
			a.FetchUrlExpiration = pbutil.MustTimestampProto(exp)
		} else {
			url, exp, err := s.contentServer.GenerateSignedURL(ctx, requestHost, a.Name)
			if err != nil {
				return err
			}

			a.FetchUrl = url
			a.FetchUrlExpiration = pbutil.MustTimestampProto(exp)
		}
	}
	return nil
}
