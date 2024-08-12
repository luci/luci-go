// Copyright 2023 The LUCI Authors.
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

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/baselines"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

const noPermissionsError = "Caller does not have permission to query new test variants"

// validateQueryNewTestVariantsRequest ensures that:
//  1. Arguments are valid
//  2. The caller has permission.
//  3. The baseline is ready for use. A baseline is not ready state if the
//     baseline identifier is in the Baseline table and lastUpdated is within 72 hours.
func validateQueryNewTestVariantsRequest(ctx context.Context, req *pb.QueryNewTestVariantsRequest) error {
	// Invocation and BaselineID provided must be formatted correctly.
	invID, err := pbutil.ParseInvocationName(req.Invocation)
	if err != nil {
		return appstatus.Error(codes.InvalidArgument, errors.Annotate(err, "invocation").Err().Error())
	}

	project, _, err := pbutil.ParseBaselineName(req.Baseline)
	if err != nil {
		return appstatus.Error(codes.InvalidArgument, errors.Annotate(err, "baseline").Err().Error())
	}

	invRealm, err := invocations.ReadRealm(ctx, invocations.ID(invID))
	if err != nil {
		// If the invocation does not exist, we mask the error with permission
		// denied to avoid leaking resource existence.
		if appstatus.Code(err) == codes.NotFound {
			return appstatus.Errorf(codes.PermissionDenied, noPermissionsError)
		} else {
			return err
		}
	}

	// Caller must have "resultdb.baselines.get" and "resultdb.testResults.list"
	// Context is already set to spanner read context, and permissions.VerifyInvocationByName()
	// will create a nested read context which is not permitted.
	switch allowed, err := auth.HasPermission(ctx, rdbperms.PermListTestResults, invRealm, nil); {
	case err != nil:
		return err
	case !allowed:
		return errors.Annotate(appstatus.Error(codes.PermissionDenied, noPermissionsError), "error1").Err()
	}

	baselineRealm := realms.Join(project, realms.ProjectRealm)
	switch allowed, err := auth.HasPermission(ctx, rdbperms.PermGetBaseline, baselineRealm, nil); {
	case err != nil:
		return err
	case !allowed:
		return errors.Annotate(appstatus.Error(codes.PermissionDenied, noPermissionsError), "error2").Err()
	}

	return nil
}

func checkBaselineStatus(ctx context.Context, project, baseline string) (isReady bool, err error) {
	// If the baseline identifier is in the Baselines table, ensure that lastUpdated
	// is older than 72 hours.
	b, err := baselines.Read(ctx, project, baseline)
	if err != nil {
		// If the baseline is not found, it could've been ejected from the Baseline table
		// meaning that it hasn't been marked for submission in a while and should not be
		// used for new test calculation.
		if err == baselines.NotFound {
			return false, nil
		} else {
			return false, err
		}
	}

	// If it's spinning up, it's not ready.
	return !b.IsSpinningUp(clock.Now(ctx)), err
}

// findNewTests calculates the difference between test variants for the baseline and test variants from
// the invocation in the request.
func findNewTests(ctx context.Context, baselineProject, baselineID string, allInvs invocations.IDSet) ([]*pb.QueryNewTestVariantsResponse_NewTestVariant, error) {
	if len(allInvs) == 0 {
		return nil, nil
	}

	// Status 5 == SKIPPED.
	st := spanner.NewStatement(`
		SELECT DISTINCT
			tr.TestId,
			tr.VariantHash,
		FROM (
			SELECT
				TestId,
				VariantHash,
				Status,
			FROM TestResults
			WHERE Status != 5 AND InvocationId IN UNNEST(@allInvs)
		) tr
		LEFT JOIN (
			SELECT
				TestId,
				VariantHash,
			FROM BaselineTestVariants b
			WHERE BaselineId = @baselineID AND Project = @baselineProject
		) b
		ON b.TestId = tr.TestId AND b.VariantHash = tr.VariantHash
		WHERE b.TestId IS NULL
		ORDER BY tr.TestId, tr.VariantHash
		LIMIT 10000
	`)

	st.Params = spanutil.ToSpannerMap(map[string]any{
		"baselineID":      baselineID,
		"baselineProject": baselineProject,
		"allInvs":         allInvs,
	})
	it := span.Query(ctx, st)

	res := []*pb.QueryNewTestVariantsResponse_NewTestVariant{}
	err := it.Do(func(r *spanner.Row) error {
		var testID, variantHash string
		err := r.Columns(&testID, &variantHash)

		if err != nil {
			return errors.Annotate(err, "read new test variant row").Err()
		}

		tv := &pb.QueryNewTestVariantsResponse_NewTestVariant{
			TestId:      testID,
			VariantHash: variantHash,
		}
		res = append(res, tv)
		return nil
	})

	return res, err
}

// findAllInvocations searches for all included invocations.
func findAllInvocations(ctx context.Context, invIDs invocations.IDSet) (invocations.IDSet, error) {
	invs, err := graph.Reachable(ctx, invIDs)
	if err != nil {
		return nil, err
	}
	rInvs := invocations.NewIDSet()
	for invID, inv := range invs.Invocations {
		if !inv.HasTestResults {
			continue
		}
		rInvs.Add(invID)
	}
	return rInvs, nil
}

// QueryNewTestVariants implements pb.ResultDBServer.
func (s *resultDBServer) QueryNewTestVariants(ctx context.Context, req *pb.QueryNewTestVariantsRequest) (*pb.QueryNewTestVariantsResponse, error) {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	err := validateQueryNewTestVariantsRequest(ctx, req)
	if err != nil {
		return nil, errors.Annotate(err, "new test variants").Err()
	}

	project, baselineID := baselines.MustParseBaselineName(req.Baseline)
	invID := invocations.MustParseName(req.Invocation)

	isReady, err := checkBaselineStatus(ctx, project, baselineID)
	if err != nil {
		return nil, errors.Annotate(err, "baseline status").Err()
	}
	if !isReady {
		return &pb.QueryNewTestVariantsResponse{
			IsBaselineReady: false,
		}, nil
	}

	// The response from graph.Reachable() should also include the invocation being
	// passed to it.
	rInvs, err := findAllInvocations(ctx, invocations.NewIDSet(invID))
	if err != nil {
		return nil, errors.Annotate(err, "failed to read the reachable invocations").Err()
	}

	nt, err := findNewTests(ctx, project, baselineID, rInvs)
	if err != nil {
		return nil, errors.Annotate(err, "failed to query for new test variants").Err()
	}

	return &pb.QueryNewTestVariantsResponse{
		NewTestVariants: nt,
		IsBaselineReady: isReady,
	}, nil
}
