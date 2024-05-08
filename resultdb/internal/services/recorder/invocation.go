// Copyright 2019 The LUCI Authors.
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

package recorder

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tokens"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/invocationspb"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	day = 24 * time.Hour

	// Delete Invocations row after this duration since invocation creation.
	invocationExpirationDuration = 2 * 365 * day // 2 y

	// By default, finalize the invocation 2d after creation if it is still
	// incomplete.
	defaultInvocationDeadlineDuration = 2 * day

	// The maximum amount of time for an invocation.
	// This is the same as the BUILD_TIMEOUT
	// https://source.chromium.org/chromium/infra/infra/+/main:appengine/cr-buildbucket/model.py;l=28;drc=e6d97dc362dd4a412fc7b07da0c4df53f2940a80
	// https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/buildbucket/appengine/model/build.go;l=53
	maxInvocationDeadlineDuration = 5 * day
)

// invocationTokenKind generates and validates tokens issued to authorize
// updating a given invocation.
var invocationTokenKind = tokens.TokenKind{
	Algo:      tokens.TokenAlgoHmacSHA256,
	SecretKey: "invocation_tokens_secret",
	Version:   1,
}

// generateInvocationToken generates an update token for a given invocation.
func generateInvocationToken(ctx context.Context, invID invocations.ID) (string, error) {
	// The token should last as long as a build is allowed to run.
	// Buildbucket has a max of 2 days, so one week should be enough even
	// for other use cases.
	return invocationTokenKind.Generate(ctx, []byte(invID), nil, 7*day) // One week.
}

// validateInvocationToken validates an update token for a given invocation,
// returning an error if the token is invalid, nil otherwise.
func validateInvocationToken(ctx context.Context, token string, invID invocations.ID) error {
	_, err := invocationTokenKind.Validate(ctx, token, []byte(invID))
	return err
}

// mutateInvocation checks if the invocation can be mutated and also
// finalizes the invocation if it's deadline is exceeded.
// If the invocation is active, continue with the other mutation(s) in f.
func mutateInvocation(ctx context.Context, id invocations.ID, f func(context.Context) error) error {
	var retErr error

	token, err := extractUpdateToken(ctx)
	if err != nil {
		return err
	}
	if err := validateInvocationToken(ctx, token, id); err != nil {
		return appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		state, err := invocations.ReadState(ctx, id)
		switch {
		case err != nil:
			return err

		case state != pb.Invocation_ACTIVE:
			return appstatus.Errorf(codes.FailedPrecondition, "%s is not active", id.Name())
		}

		return f(ctx)
	})

	if err != nil {
		retErr = err
	}
	return retErr
}

func extractUpdateToken(ctx context.Context) (string, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	token := md.Get(pb.UpdateTokenMetadataKey)
	switch {
	case len(token) == 0:
		return "", appstatus.Errorf(codes.Unauthenticated, "missing %s metadata value in the request", pb.UpdateTokenMetadataKey)

	case len(token) > 1:
		return "", appstatus.Errorf(codes.InvalidArgument, "expected exactly one %s metadata value, got %d", pb.UpdateTokenMetadataKey, len(token))

	default:
		return token[0], nil
	}
}

// rowOfInvocation returns Invocation row values to be inserted to create the
// invocation.
// inv.CreateTime, inv.FinalizeStartTime and inv.FinalizeTime are ignored
// and set by the implementation to spanner.CommitTime as appropriate.
func (s *recorderServer) rowOfInvocation(ctx context.Context, inv *pb.Invocation, createRequestID string) map[string]any {
	now := clock.Now(ctx).UTC()
	row := map[string]any{
		"InvocationId": invocations.MustParseName(inv.Name),
		"ShardId":      mathrand.Intn(ctx, invocations.Shards),
		"State":        inv.State,
		"Realm":        inv.Realm,
		"CreatedBy":    inv.CreatedBy,

		"InvocationExpirationTime":          now.Add(invocationExpirationDuration),
		"ExpectedTestResultsExpirationTime": now.Add(s.ExpectedResultsExpiration),

		"CreateTime": spanner.CommitTimestamp,
		"Deadline":   inv.Deadline,

		"Tags":              inv.Tags,
		"ProducerResource":  inv.ProducerResource,
		"IsExportRoot":      spanner.NullBool{Valid: inv.IsExportRoot, Bool: inv.IsExportRoot},
		"BigQueryExports":   inv.BigqueryExports,
		"Properties":        spanutil.Compressed(pbutil.MustMarshal(inv.Properties)),
		"InheritSources":    spanner.NullBool{Valid: inv.SourceSpec != nil, Bool: inv.SourceSpec.GetInherit()},
		"Sources":           spanutil.Compressed(pbutil.MustMarshal(inv.SourceSpec.GetSources())),
		"IsSourceSpecFinal": spanner.NullBool{Valid: inv.IsSourceSpecFinal, Bool: inv.IsSourceSpecFinal},
		"BaselineId":        inv.BaselineId,
		"TestInstruction":   spanutil.Compressed(pbutil.MustMarshal(inv.TestInstruction)),
		"StepInstructions":  spanutil.Compressed(pbutil.MustMarshal(inv.StepInstructions)),
	}

	// Wrap into luci.resultdb.internal.invocations.ExtendedProperties so that
	// it can be serialized as a single value to spanner.
	internalExtendedProperties := &invocationspb.ExtendedProperties{
		ExtendedProperties: inv.ExtendedProperties,
	}
	row["ExtendedProperties"] = spanutil.Compressed(pbutil.MustMarshal(internalExtendedProperties))

	if inv.State == pb.Invocation_FINALIZING || inv.State == pb.Invocation_FINALIZED {
		// Invocation immediately transitioning to finalizing/finalized.
		row["FinalizeStartTime"] = spanner.CommitTimestamp
	}
	if inv.State == pb.Invocation_FINALIZED {
		// Invocation immediately finalized.
		row["FinalizeTime"] = spanner.CommitTimestamp
	}

	if createRequestID != "" {
		row["CreateRequestId"] = createRequestID
	}

	return row
}
