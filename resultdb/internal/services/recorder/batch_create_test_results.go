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
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/resultcount"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

var (
	testResultsUploadedWithoutModuleIDValidationCounter = metric.NewCounter(
		"resultdb/recorder/test_results_uploaded_without_module_id_validation",
		"The number of test results uploaded without the parent invocation having a module_id set, by LUCI Realm.",
		nil,
		// The LUCI Realm.
		field.String("realm"))
)

// BatchCreateTestResults implements pb.RecorderServer.
func (s *recorderServer) BatchCreateTestResults(ctx context.Context, in *pb.BatchCreateTestResultsRequest) (*pb.BatchCreateTestResultsResponse, error) {
	// Per AIP-211, perform authorisation checks before request validation.
	if err := verifyBatchCreateTestResultsPermissions(ctx, in); err != nil {
		return nil, err
	}

	now := clock.Now(ctx).UTC()
	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateBatchCreateTestResultsRequest(ctx, in, cfg, now); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	if in.Invocation != "" {
		// Use legacy implementation.
		return batchCreateResultsInInvocation(ctx, in)
	}
	// Use work units implementation.
	return batchCreateResultsInWorkUnits(ctx, in)
}

// verifyBatchCreateTestResultsPermissions verifies the caller has provided
// the update-token(s) sufficient to create the given test results.
func verifyBatchCreateTestResultsPermissions(ctx context.Context, req *pb.BatchCreateTestResultsRequest) (err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/recorder.verifyBatchCreateTestResultsPermissions",
		attribute.Int("cr.dev.count", len(req.Requests)),
	)
	defer func() { tracing.End(ts, err) }()

	// Three cases are allowed:
	// 1. Parent is specified on the batch request. The child request, if it sets Parent, must set the same value.
	//    Invocation is not specified everywhere.
	// 2. Invocation is specified on the batch request. The child request, if it sets Invocation, must set the same value.
	//    Parent is not specified everywhere.
	// 3. Parent is specified on the child request only. Invocation is not specified anywhere.

	var state string
	if req.Parent != "" {
		// Case 1.
		wuID, err := workunits.ParseName(req.Parent)
		if err != nil {
			return appstatus.BadRequest(errors.Fmt("parent: %w", err))
		}
		if req.Invocation != "" {
			return appstatus.BadRequest(errors.New("invocation: must not be specified if parent is specified"))
		}
		state = workUnitUpdateTokenState(wuID)
	}
	if req.Invocation != "" {
		// Case 2. Go into the legacy validation path.
		return verifyBatchCreateTestResultsPermissionLegacy(ctx, req)
	}

	if err := pbutil.ValidateBatchRequestCountAndSize(req.Requests); err != nil {
		return appstatus.BadRequest(errors.Fmt("requests: %w", err))
	}

	var rootInvocationID rootinvocations.ID
	for i, r := range req.Requests {
		if r.Invocation != "" {
			if req.Parent != "" {
				return appstatus.BadRequest(errors.Fmt("requests[%d]: invocation: must not be set if `parent` is set on top-level batch request", i))
			} else {
				return appstatus.BadRequest(errors.Fmt("requests[%d]: invocation: may not be set at the request-level unless also set at the batch-level", i))
			}
		}
		if req.Parent != "" {
			// Case 1. Expect the `parent` field set on the child request, if set, to match that on the batch request.
			if err := emptyOrEqual("parent", r.Parent, req.Parent); err != nil {
				return appstatus.BadRequest(errors.Fmt("requests[%d]: %w", i, err))
			}
		} else {
			// Case 3. Parent is set on a per-request basis.
			parentID, err := workunits.ParseName(r.Parent)
			if err != nil {
				return appstatus.BadRequest(errors.Fmt("requests[%d]: parent: %w", i, err))
			}

			// Check all root invocations are the same. This can generate more helpful errors
			// than checking the token states are equal directly.
			if i == 0 {
				rootInvocationID = parentID.RootInvocationID
			} else if rootInvocationID != parentID.RootInvocationID {
				return appstatus.BadRequest(errors.Fmt("requests[%d]: parent: all test results in a batch must belong to the same root invocation; got %q, want %q", i, parentID.RootInvocationID.Name(), rootInvocationID.Name()))
			}

			s := workUnitUpdateTokenState(parentID)
			if i == 0 {
				state = s
			} else if state != s {
				return appstatus.BadRequest(errors.Fmt("requests[%d]: parent: work unit %q requires a different update token to request[0]'s %q, but this RPC only accepts one update token", i, parentID.Name(), req.Requests[0].Parent))
			}
		}
	}

	token, err := extractUpdateToken(ctx)
	if err != nil {
		return err
	}
	if err := validateWorkUnitUpdateTokenForState(ctx, token, state); err != nil {
		return appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}
	return nil
}

func verifyBatchCreateTestResultsPermissionLegacy(ctx context.Context, req *pb.BatchCreateTestResultsRequest) error {
	if err := pbutil.ValidateInvocationName(req.Invocation); err != nil {
		return appstatus.BadRequest(errors.Fmt("invocation: %w", err))
	}
	// TODO: Get rid of the requirement to support an empty requests collection if we can.
	if len(req.Requests) > 0 {
		if err := pbutil.ValidateBatchRequestCountAndSize(req.Requests); err != nil {
			return appstatus.BadRequest(errors.Fmt("requests: %w", err))
		}
	}
	for i, r := range req.Requests {
		if err := emptyOrEqual("invocation", r.Invocation, req.Invocation); err != nil {
			return appstatus.BadRequest(errors.Fmt("requests[%d]: %w", i, err))
		}
		if r.Parent != "" {
			return appstatus.BadRequest(errors.Fmt("requests[%d]: parent: must not be set if `invocation` is set on top-level batch request", i))
		}
	}

	token, err := extractUpdateToken(ctx)
	if err != nil {
		return err
	}
	id := invocations.MustParseName(req.Invocation)
	if err := validateInvocationToken(ctx, token, id); err != nil {
		return appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}
	return nil
}

func validateBatchCreateTestResultsRequest(ctx context.Context, req *pb.BatchCreateTestResultsRequest, cfg *config.CompiledServiceConfig, now time.Time) (err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/recorder.validateBatchCreateTestResultsRequest")
	defer func() { tracing.End(ts, err) }()

	// Parent, Invocation and Request length is already validated by
	// verifyBatchCreateTestResultsPermissions.

	// If we are operating on work units, enforce stricter validation.
	strictValidation := req.Invocation == ""

	if strictValidation && req.RequestId == "" {
		// Request ID is required to ensure requests are treated idempotently
		// in case of inevitable retries.
		return errors.Fmt("request_id: unspecified (please provide a per-request UUID to ensure idempotence)")
	}
	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Fmt("request_id: %w", err)
	}

	type Key struct {
		testID   string
		resultID string
	}
	keyToIndex := make(map[Key]int)

	for i, r := range req.Requests {
		if err := emptyOrEqual("request_id", r.RequestId, req.RequestId); err != nil {
			return errors.Fmt("requests[%d]: %w", i, err)
		}
		if err := validateTestResult(now, cfg, r.TestResult); err != nil {
			return errors.Fmt("requests[%d]: test_result: %w", i, err)
		}

		var testID string
		if r.TestResult.TestIdStructured != nil {
			testID = pbutil.EncodeTestID(pbutil.ExtractBaseTestIdentifier(r.TestResult.TestIdStructured))
		} else {
			testID = r.TestResult.TestId
		}
		key := Key{
			testID:   testID,
			resultID: r.TestResult.ResultId,
		}
		if lastIndex, ok := keyToIndex[key]; ok {
			// Duplicated results.
			return errors.Fmt("requests[%d]: a test result with the same test ID and result ID already exists at request[%d]; testID %q, resultID %q", i, lastIndex, key.testID, key.resultID)
		}
		keyToIndex[key] = i
	}
	return nil
}

func emptyOrEqual(name, actual, expected string) error {
	switch actual {
	case "", expected:
		return nil
	}
	return errors.Fmt("%s: must be either empty or equal to %q, but got %q", name, expected, actual)
}

func batchCreateResultsInWorkUnits(ctx context.Context, in *pb.BatchCreateTestResultsRequest) (rsp *pb.BatchCreateTestResultsResponse, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/recorder.batchCreateResultsInWorkUnits")
	defer func() { tracing.End(ts, err) }()

	// The common work unit set at the batch-request level (if any).
	var commonParentID workunits.ID
	if in.Parent != "" {
		commonParentID = workunits.MustParseName(in.Parent)
	}

	ret := &pb.BatchCreateTestResultsResponse{
		TestResults: make([]*pb.TestResult, len(in.Requests)),
	}
	// The test results to insert.
	testResultInserts := make([]*spanner.Mutation, len(in.Requests))

	// All work units referenced in the batch.
	parents := workunits.NewIDSet()

	// The number of test results inserted per invocation.
	testResultsPerInvocation := make(map[invocations.ID]int64)

	// The test identifier of each test result.
	testResultTestIdentifiers := make([]*pb.TestIdentifier, len(in.Requests))

	// Prepare the test results we want to write outside the read-update
	// transaction to minimise the transaction duration.
	for i, r := range in.Requests {
		parentID := commonParentID
		if parentID == (workunits.ID{}) {
			// If the work unit is not set on the batch request, it must be set at the child request level.
			parentID = workunits.MustParseName(r.Parent)
		}

		legacyInvID := parentID.LegacyInvocationID()
		tr, mutation, err := insertTestResult(legacyInvID, in.RequestId, r.TestResult)
		if err != nil {
			return nil, err
		}
		ret.TestResults[i] = tr
		testResultInserts[i] = mutation

		parents.Add(parentID)

		testResultsPerInvocation[legacyInvID]++
		testResultTestIdentifiers[i] = extractTestResultTestIdentifier(tr)
	}

	var insertsByRealm map[string]int
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		// Reset at the start of each transaction as this context may be retried.
		insertsByRealm = make(map[string]int)

		parentsSlice := parents.SortedByRowID()
		parentInfos, err := workunits.ReadSummaryInfos(ctx, parentsSlice)
		if err != nil {
			// NotFound appstatus error or internal error.
			return err
		}

		// Verify assumptions:
		// - the parent work unit is active.
		// - the test results are in the correct module for each work unit.
		for i, r := range in.Requests {
			parentID := commonParentID
			if parentID == (workunits.ID{}) {
				// If the work unit is not set on the batch request, it must be set at the child request level.
				parentID = workunits.MustParseName(r.Parent)
			}
			parentInfo, ok := parentInfos[parentID]
			if !ok {
				// If the parent was not read, we should have errored out already above.
				panic("logic error: parentID not in parentInfos")
			}
			if parentInfo.FinalizationState != pb.WorkUnit_ACTIVE {
				return appstatus.Errorf(codes.FailedPrecondition, "requests[%d]: parent %q is not active", i, parentID.Name())
			}
			const strictValidation = true
			if err := validateUploadAgainstWorkUnitModule(testResultTestIdentifiers[i], parentInfo.ModuleID, strictValidation); err != nil {
				return appstatus.Errorf(codes.FailedPrecondition, "requests[%d]: test_result: %s", i, err)
			}
			// Count inserts by realm
			insertsByRealm[parentInfo.Realm]++
		}

		// Insert the test results.
		// TODO: These are currently blind InsertOrUpdate mutations. These
		// should be made Insert mutations and if the transaction aborts
		// due to a test result already existing, we should validate if they
		// were created by a request with this request_id (and succeed)
		// or a different request_id (and return an already exists error).
		span.BufferWrite(ctx, testResultInserts...)

		// Increment the number of test results per invocation.
		err = resultcount.BatchIncrementTestResultCount(ctx, testResultsPerInvocation)
		if err != nil {
			return errors.Fmt("increment test result counts: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	for realm, count := range insertsByRealm {
		spanutil.IncRowCount(ctx, count, spanutil.TestResults, spanutil.Inserted, realm)
	}
	return ret, nil
}

func batchCreateResultsInInvocation(ctx context.Context, in *pb.BatchCreateTestResultsRequest) (rsp *pb.BatchCreateTestResultsResponse, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/recorder.batchCreateResultsInInvocation")
	defer func() { tracing.End(ts, err) }()

	invID := invocations.MustParseName(in.Invocation)

	ret := &pb.BatchCreateTestResultsResponse{
		TestResults: make([]*pb.TestResult, len(in.Requests)),
	}

	// Prepare the test results we want to write outside the read-update
	// transaction to minimise the transaction duration.
	testResultInserts := make([]*spanner.Mutation, len(in.Requests))
	var commonPrefix string
	varUnion := stringset.New(0)
	// The test identifier of each test result.
	testResultTestIdentifiers := make([]*pb.TestIdentifier, len(in.Requests))
	for i, r := range in.Requests {
		tr, mutation, err := insertTestResult(invID, in.RequestId, r.TestResult)
		if err != nil {
			return nil, err
		}
		ret.TestResults[i] = tr
		testResultInserts[i] = mutation
		if i == 0 {
			commonPrefix = ret.TestResults[i].TestId
		} else {
			commonPrefix = longestCommonPrefix(commonPrefix, ret.TestResults[i].TestId)
		}
		varUnion.AddAll(pbutil.VariantToStrings(ret.TestResults[i].GetVariant()))
		testResultTestIdentifiers[i] = extractTestResultTestIdentifier(tr)
	}

	var realm string
	_, err = mutateInvocation(ctx, invID, func(ctx context.Context) error {
		span.BufferWrite(ctx, testResultInserts...)
		eg, ctx := errgroup.WithContext(ctx)
		eg.Go(func() (err error) {
			var invCommonTestIdPrefix spanner.NullString
			var invVars []string
			var moduleName spanner.NullString
			var moduleScheme spanner.NullString
			var moduleVariant *pb.Variant
			if err = invocations.ReadColumns(ctx, invID, map[string]any{
				"Realm":                  &realm,
				"CommonTestIDPrefix":     &invCommonTestIdPrefix,
				"TestResultVariantUnion": &invVars,
				"ModuleName":             &moduleName,
				"ModuleScheme":           &moduleScheme,
				"ModuleVariant":          &moduleVariant,
			}); err != nil {
				return
			}

			var expectedModule *pb.ModuleIdentifier
			if moduleName.Valid && moduleScheme.Valid {
				expectedModule = &pb.ModuleIdentifier{
					ModuleName:    moduleName.StringVal,
					ModuleScheme:  moduleScheme.StringVal,
					ModuleVariant: moduleVariant,
				}
			}

			// Ensure the module of each result matches the expected value.
			for i, testIdentifier := range testResultTestIdentifiers {
				const strictValidation = false
				if err := validateUploadAgainstWorkUnitModule(testIdentifier, expectedModule, strictValidation); err != nil {
					return appstatus.Errorf(codes.FailedPrecondition, "requests[%d]: test_result: %s", i, err)
				}
				if expectedModule == nil {
					testResultsUploadedWithoutModuleIDValidationCounter.Add(ctx, 1, realm)
				}
			}

			newPrefix := commonPrefix
			if !invCommonTestIdPrefix.IsNull() {
				newPrefix = longestCommonPrefix(invCommonTestIdPrefix.String(), commonPrefix)
			}
			varUnion.AddAll(invVars)

			if invCommonTestIdPrefix.String() != newPrefix || varUnion.Len() > len(invVars) {
				span.BufferWrite(ctx, spanutil.UpdateMap("Invocations", map[string]any{
					"InvocationId":           invID,
					"CommonTestIDPrefix":     newPrefix,
					"TestResultVariantUnion": varUnion.ToSortedSlice(),
				}))
			}

			return
		})
		eg.Go(func() error {
			return resultcount.IncrementTestResultCount(ctx, invID, int64(len(in.Requests)))
		})
		return eg.Wait()
	})
	if err != nil {
		return nil, err
	}

	spanutil.IncRowCount(ctx, len(in.Requests), spanutil.TestResults, spanutil.Inserted, realm)
	return ret, nil
}

func insertTestResult(invID invocations.ID, requestID string, body *pb.TestResult) (*pb.TestResult, *spanner.Mutation, error) {
	// create a copy of the input message with the OUTPUT_ONLY field(s) to be used in
	// the response
	ret := proto.Clone(body).(*pb.TestResult)

	if ret.TestIdStructured != nil {
		// Use TestVariantIdentifier to set TestId and Variant (OUTPUT_ONLY fields).
		// Also set the OUTPUT_ONLY fields in TestVariantIdentiifer.
		ret.TestId = pbutil.TestIDFromStructuredTestIdentifier(ret.TestIdStructured)
		ret.Variant = pbutil.VariantFromStructuredTestIdentifier(ret.TestIdStructured)
		pbutil.PopulateStructuredTestIdentifierHashes(ret.TestIdStructured)
	} else {
		// Legacy test uploader. Populate TestIdStructured from TestId and Variant
		// so that this RPC returns the same response as ListTestResults.
		var err error
		ret.TestIdStructured, err = pbutil.ParseStructuredTestIdentifierForOutput(ret.TestId, ret.Variant)
		if err != nil {
			// This should not happen, the test identifier should already have been validated.
			return nil, nil, errors.Fmt("parse test identifier: %w", err)
		}
	}
	ret.VariantHash = pbutil.VariantHash(ret.Variant)

	if invID.IsWorkUnit() {
		wuID := workunits.MustParseLegacyInvocationID(invID)
		ret.Name = pbutil.TestResultName(string(wuID.RootInvocationID), wuID.WorkUnitID, ret.TestId, ret.ResultId)
	} else {
		ret.Name = pbutil.LegacyTestResultName(string(invID), ret.TestId, ret.ResultId)
	}

	// handle values for nullable columns
	var runDuration spanner.NullInt64
	if ret.Duration != nil {
		runDuration.Int64 = pbutil.MustDuration(ret.Duration).Microseconds()
		runDuration.Valid = true
	}

	if ret.StatusV2 != pb.TestResult_STATUS_UNSPECIFIED {
		// Populate v1 status from v2 status fields.
		ret.Status, ret.Expected = pbutil.TestStatusV1FromV2(ret.StatusV2, ret.FailureReason.GetKind(), ret.FrameworkExtensions.GetWebTest())
	} else {
		status, failureKind, webTest := pbutil.TestStatusV2FromV1(ret.Status, ret.Expected)

		// Populate v2 status fields from v1 status.
		ret.StatusV2 = status
		if status == pb.TestResult_FAILED {
			if ret.FailureReason == nil {
				ret.FailureReason = &pb.FailureReason{}
			}
			ret.FailureReason.Kind = failureKind
		}
		if webTest != nil {
			if ret.FrameworkExtensions == nil {
				ret.FrameworkExtensions = &pb.FrameworkExtensions{}
			}
			ret.FrameworkExtensions.WebTest = webTest
		}
	}

	row := map[string]any{
		"InvocationId":    invID,
		"TestId":          ret.TestId,
		"ResultId":        ret.ResultId,
		"Variant":         ret.Variant,
		"VariantHash":     ret.VariantHash,
		"CommitTimestamp": spanner.CommitTimestamp,
		"IsUnexpected":    spanner.NullBool{Bool: true, Valid: !ret.Expected},
		"Status":          ret.Status,
		"StatusV2":        ret.StatusV2,
		"SummaryHTML":     spanutil.Compressed(ret.SummaryHtml),
		"StartTime":       ret.StartTime,
		"RunDurationUsec": runDuration,
		"Tags":            ret.Tags,
	}
	if ret.SkipReason != pb.SkipReason_SKIP_REASON_UNSPECIFIED {
		// Unspecified is mapped to NULL, so only write if we have some other value.
		row["SkipReason"] = ret.SkipReason
	}
	if ret.TestMetadata != nil {
		row["TestMetadata"] = spanutil.Compressed(pbutil.MustMarshal(ret.TestMetadata))
	}
	if ret.FailureReason != nil {
		// Normalise the failure reason. This handles legacy uploaders which only set
		// PrimaryErrorMessage by pushing it into the Errors collection instead.
		testresults.NormaliseFailureReason(ret.FailureReason)
		row["FailureReason"] = spanutil.Compressed(pbutil.MustMarshal(ret.FailureReason))

		// Populate output only fields after marshalling, as we don't want to store those.
		testresults.PopulateFailureReasonOutputOnlyFields(ret.FailureReason)
	}
	if ret.Properties != nil {
		row["Properties"] = spanutil.Compressed(pbutil.MustMarshal(ret.Properties))
	}
	if ret.SkippedReason != nil {
		row["SkippedReason"] = spanutil.Compressed(pbutil.MustMarshal(ret.SkippedReason))
	}
	if ret.FrameworkExtensions != nil {
		row["FrameworkExtensions"] = spanutil.Compressed(pbutil.MustMarshal(ret.FrameworkExtensions))
	}
	mutation := spanner.InsertOrUpdateMap("TestResults", spanutil.ToSpannerMap(row))
	return ret, mutation, nil
}

func longestCommonPrefix(str1, str2 string) string {
	for i := 0; i < len(str1) && i < len(str2); i++ {
		if str1[i] != str2[i] {
			return str1[:i]
		}
	}
	if len(str1) <= len(str2) {
		return str1
	}
	return str2
}

// validateTestResult returns a non-nil error if msg is invalid.
func validateTestResult(now time.Time, cfg *config.CompiledServiceConfig, tr *pb.TestResult) error {
	validateToScheme := func(testID pbutil.BaseTestIdentifier) error {
		return validateTestIDToScheme(cfg, testID)
	}
	if err := pbutil.ValidateTestResult(now, validateToScheme, tr); err != nil {
		return err
	}
	return nil
}

func validateTestIDToScheme(cfg *config.CompiledServiceConfig, testID pbutil.BaseTestIdentifier) error {
	scheme, ok := cfg.Schemes[testID.ModuleScheme]
	if !ok {
		return errors.Fmt("module_scheme: scheme %q is not a known scheme by the ResultDB deployment; see go/resultdb-schemes for instructions how to define a new scheme", testID.ModuleScheme)
	}
	return scheme.Validate(testID)
}

func extractTestResultTestIdentifier(tr *pb.TestResult) *pb.TestIdentifier {
	if tr.TestIdStructured == nil {
		panic("expected test_id_structured to be set")
	}
	return tr.TestIdStructured
}

// Validates the module of the uploaded test result or test result artifact against the module
// expected by the work unit or invocation.
func validateUploadAgainstWorkUnitModule(got *pb.TestIdentifier, expectedModuleID *pb.ModuleIdentifier, strictValidation bool) error {
	if expectedModuleID != nil {
		if got.ModuleName != expectedModuleID.ModuleName {
			return errors.Fmt("test_id_structured: module_name: does not match parent work unit module_id.module_name; got %q, want %q", got.ModuleName, expectedModuleID.ModuleName)
		}
		if got.ModuleScheme != expectedModuleID.ModuleScheme {
			return errors.Fmt("test_id_structured: module_scheme: does not match parent work unit module_id.module_scheme; got %q, want %q", got.ModuleScheme, expectedModuleID.ModuleScheme)
		}
		isLegacyID := expectedModuleID.ModuleName == "legacy"
		// For legacy test IDs, do not validate the variant as they can be overridden at the test result level.
		// For structured test IDs, the test module variant should match that specified on the work unit module.
		if !isLegacyID && !pbutil.VariantsEqual(got.ModuleVariant, expectedModuleID.ModuleVariant) {
			gotKeys, err := pbutil.VariantToJSON(got.ModuleVariant)
			if err != nil {
				gotKeys = "{<invalid>}"
			}
			wantKeys, err := pbutil.VariantToJSON(expectedModuleID.ModuleVariant)
			if err != nil {
				wantKeys = "{<invalid>}"
			}
			return errors.Fmt("test_id_structured: module_variant: does not match parent work unit module_id.module_variant; got %s, want %s", gotKeys, wantKeys)
		}
	} else {
		if strictValidation {
			return errors.Fmt("to upload test results or test result artifacts, you must set the module_id on the parent work unit first")
		} else {
			// For compatibility reasons we will let legacy and structured test IDs upload to invocations
			// with module_id unset for a time.
			// TODO(meiring): Uncomment the following check once Android has migrated to work units.
			// if got.ModuleName != "legacy" || got.ModuleScheme != "legacy" {
			// 	return errors.Fmt("test_id_structured: to upload results with structured test IDs, you must set the module_id on the parent legacy invocation first")
			// }
		}
	}
	return nil
}
