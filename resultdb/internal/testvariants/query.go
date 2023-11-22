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

package testvariants

import (
	"context"
	"fmt"
	"strings"
	"text/template"

	"cloud.google.com/go/spanner"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/resultdb/internal/exonerations"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth"
)

const (
	// resultLimitMax is the maximum number of results can be included in a test
	// variant when querying test variants. The client may specify a lower limit.
	// It is required to prevent client-caused OOMs.
	resultLimitMax     = 100
	resultLimitDefault = 10

	// DefaultResponseLimitBytes is the default soft limit on the number of bytes
	// that should be returned by a Test Variants query.
	DefaultResponseLimitBytes = 20 * 1000 * 1000 // 20 MB
)

// AllFields is a field mask that selects all TestVariant fields.
var AllFields = mask.All(&pb.TestVariant{})

// QueryMask returns mask.Mask converted from field_mask.FieldMask.
// It returns a default mask with all fields if readMask is empty.
func QueryMask(readMask *field_mask.FieldMask) (*mask.Mask, error) {
	if len(readMask.GetPaths()) == 0 {
		return AllFields, nil
	}
	return mask.FromFieldMask(readMask, &pb.TestVariant{}, false, false)
}

// AdjustResultLimit takes the given requested resultLimit and adjusts as
// necessary.
func AdjustResultLimit(resultLimit int32) int {
	switch {
	case resultLimit >= resultLimitMax:
		return resultLimitMax
	case resultLimit > 0:
		return int(resultLimit)
	default:
		return resultLimitDefault
	}
}

// ValidateResultLimit returns a non-nil error if resultLimit is invalid.
// Returns nil if resultLimit is 0.
func ValidateResultLimit(resultLimit int32) error {
	if resultLimit < 0 {
		return errors.Reason("negative").Err()
	}
	return nil
}

type AccessLevel int

const (
	// The access level is invalid; either an error occurred when checking the
	// caller's access, or the caller does not have any kind of access to data in
	// the realms of the invocations being queried.
	AccessLevelInvalid AccessLevel = iota
	// Limited access. The caller has access to metadata only, such as
	// pass/fail status of tests, test IDs and sanitised failure reasons.
	AccessLevelLimited
	// The caller has access to all data in the realms of the invocations being
	// queried.
	AccessLevelUnrestricted
)

// Query specifies test variants to fetch.
type Query struct {
	ReachableInvocations graph.ReachableInvocations
	Predicate            *pb.TestVariantPredicate
	// ResultLimit is a limit on the number of test results returned
	// per test variant. Must be postiive.
	ResultLimit int
	// PageSize is a limit on the number of test variants returned
	// per page. The actual number of test variants returned may be
	// less than this, or even zero. (It may be zero even if there are
	// more test variants in later pages). Must be positive.
	PageSize int
	// ResponseLimitBytes is the soft limit on the number of bytes returned
	// by a query. The soft limit is on JSON-serialised form of the
	// result. A page will be truncated once it exceeds this value.
	// Must be positive.
	ResponseLimitBytes int
	// Consists of test variant status, test id and variant hash.
	PageToken string
	Mask      *mask.Mask
	TestIDs   []string
	// The level of access the user has to test results and test exoneration data.
	AccessLevel AccessLevel

	decompressBuf []byte         // buffer for decompressing blobs
	params        map[string]any // query parameters
}

// trim is equivalent to q.Mask.Trim with the exception that "test_id",
// "variant_hash", and "status" are always kept intact. Those fields are needed
// to generate page tokens.
func (q *Query) trim(tv *pb.TestVariant) error {
	testID := tv.TestId
	vHash := tv.VariantHash
	status := tv.Status

	if err := q.Mask.Trim(tv); err != nil {
		return errors.Annotate(err, "error trimming fields for test variant with ID: %s, variant hash: %s", tv.TestId, tv.VariantHash).Err()
	}

	tv.TestId = testID
	tv.VariantHash = vHash
	tv.Status = status
	return nil
}

// tvResult matches the result STRUCT of a test variant from the query.
type tvResult struct {
	InvocationID    string
	ResultID        string
	IsUnexpected    spanner.NullBool
	Status          int64
	StartTime       spanner.NullTime
	RunDurationUsec spanner.NullInt64
	SummaryHTML     []byte
	FailureReason   []byte
	Tags            []string
	Properties      []byte
	SkipReason      spanner.NullInt64
}

// resultSelectColumns returns a list of columns needed to fetch `tvResult`s
// according to the fieldmask. `InvocationId`, `ResultId` and `IsUnexpected` are
// always selected.
func (q *Query) resultSelectColumns() []string {
	// Note: `InvocationId` and `ResultId` are necessary to construct
	// TestResult.Name, so they must be included.
	columnSet := stringset.NewFromSlice(
		"InvocationId",
		"ResultId",
		"IsUnexpected",
	)

	// Select extra columns depending on the mask.
	readMask := q.Mask
	if readMask.IsEmpty() {
		readMask = AllFields
	}
	selectIfIncluded := func(column string, fields ...string) {
		for _, field := range fields {
			switch inc, err := readMask.Includes(field); {
			case err != nil:
				panic(err)
			case inc != mask.Exclude:
				columnSet.Add(column)
				return
			}
		}
	}

	selectIfIncluded("Status", "results.*.result.status")
	selectIfIncluded("StartTime", "results.*.result.start_time")
	selectIfIncluded("RunDurationUsec", "results.*.result.duration")
	selectIfIncluded("SummaryHTML", "results.*.result.summary_html")
	selectIfIncluded("FailureReason", "results.*.result.failure_reason")
	selectIfIncluded("Tags", "results.*.result.tags")
	selectIfIncluded("Properties", "results.*.result.properties")
	selectIfIncluded("SkipReason", "results.*.result.skip_reason")

	return columnSet.ToSortedSlice()
}

func (q *Query) decompressText(src []byte) (string, error) {
	if len(src) == 0 {
		return "", nil
	}
	var err error
	if q.decompressBuf, err = spanutil.Decompress(src, q.decompressBuf); err != nil {
		return "", err
	}
	return string(q.decompressBuf), nil
}

// decompressProto decompresses and unmarshals src to dest. It's a noop if src
// is empty.
func (q *Query) decompressProto(src []byte, dest proto.Message) error {
	if len(src) == 0 {
		return nil
	}
	var err error
	if q.decompressBuf, err = spanutil.Decompress(src, q.decompressBuf); err != nil {
		return err
	}
	return proto.Unmarshal(q.decompressBuf, dest)
}

func (q *Query) toTestResultProto(r *tvResult, testID string) (*pb.TestResult, error) {
	tr := &pb.TestResult{
		ResultId: r.ResultID,
		Status:   pb.TestStatus(r.Status),
	}
	tr.Name = pbutil.TestResultName(
		string(invocations.IDFromRowID(r.InvocationID)), testID, r.ResultID)

	if r.StartTime.Valid {
		tr.StartTime = pbutil.MustTimestampProto(r.StartTime.Time)
	}
	testresults.PopulateExpectedField(tr, r.IsUnexpected)
	testresults.PopulateDurationField(tr, r.RunDurationUsec)
	testresults.PopulateSkipReasonField(tr, r.SkipReason)

	var err error
	if tr.SummaryHtml, err = q.decompressText(r.SummaryHTML); err != nil {
		return nil, err
	}

	if len(r.FailureReason) != 0 {
		// Don't initialize FailureReason when r.FailureReason is empty so
		// it won't produce {"failureReason": {}} when serialized to JSON.
		tr.FailureReason = &pb.FailureReason{}

		if err := q.decompressProto(r.FailureReason, tr.FailureReason); err != nil {
			return nil, err
		}
	}
	if len(r.Properties) != 0 {
		// Don't initialize properties when r.Properties is empty so
		// it won't produce {"Properties": {}} when serialized to JSON.
		tr.Properties = &structpb.Struct{}

		if err := q.decompressProto(r.Properties, tr.Properties); err != nil {
			return nil, err
		}
	}

	// Populate Tags.
	tr.Tags = make([]*pb.StringPair, len(r.Tags))
	for i, p := range r.Tags {
		tr.Tags[i] = pbutil.StringPairFromStringUnvalidated(p)
	}

	return tr, nil
}

// populateSources populates the sources tested by each test variant, by
// setting its source_id and ensuring a corresponding entry exists in
// the distinctSources map. The sources tested come from the invocation of
// an *arbitrary* test result in the test variant.
func (q *Query) populateSources(tv *pb.TestVariant, distinctSources map[string]*pb.Sources) error {
	if len(tv.Results) == 0 {
		return nil
	}
	// Find the invocation for one of the test results in the test variant.
	tr := tv.Results[0].Result
	invID, _, _, err := pbutil.ParseTestResultName(tr.Name)
	if err != nil {
		return err
	}
	// Look up that invocation's sources.
	inv, ok := q.ReachableInvocations.Invocations[invocations.ID(invID)]
	if !ok {
		return errors.Reason("test result in response referenced an unreachable invocation").Err()
	}
	sources := q.ReachableInvocations.Sources[inv.SourceHash]
	if sources != nil {
		// Associate those sources with the test variant.
		tv.SourcesId = inv.SourceHash.String()
		distinctSources[inv.SourceHash.String()] = sources
	} else {
		// No sources available.
		tv.SourcesId = ""
	}
	return nil
}

// toLimitedData limits the given TestVariant (and its test results and test
// exonerations) to the fields allowed when the caller only has listLimited
// permissions for test results and test exonerations. If the caller has
// permission to get test results / test exonerations in the realm of the
// immediate parent invocation, the test result / test exoneration will not be
// restricted.
func (q *Query) toLimitedData(ctx context.Context, tv *pb.TestVariant,
	resultPerms map[string]bool, exonerationPerms map[string]bool) error {
	shouldMaskMetadata := true
	shouldMaskVariant := true
	for _, resultBundle := range tv.Results {
		tr := resultBundle.Result
		invID, _, _, err := pbutil.ParseTestResultName(tr.Name)
		if err != nil {
			return err
		}

		// Get the immediate invocation to which this test result belongs.
		reachableInv, ok := q.ReachableInvocations.Invocations[invocations.ID(invID)]
		if !ok {
			return fmt.Errorf("error finding realm: invocation %s not found", invID)
		}

		// Check if the caller has permission to get test results in the immediate
		// invocation's realm.
		allowedResult, ok := resultPerms[reachableInv.Realm]
		if !ok {
			// The test result permission in this realm hasn't been checked before.
			allowedResult, err = auth.HasPermission(ctx,
				rdbperms.PermGetTestResult, reachableInv.Realm, nil)
			if err != nil {
				return errors.Annotate(err,
					"error checking permission for test result data restriction").Err()
			}
			// Record whether the caller has permission to get test results in the
			// realm so the HasPermission result can be re-used.
			resultPerms[reachableInv.Realm] = allowedResult
		}

		if allowedResult {
			shouldMaskMetadata = false
			shouldMaskVariant = false
		} else {
			// Restrict test result data.
			if err := testresults.ToLimitedData(ctx, tr); err != nil {
				return err
			}
		}
	}

	if shouldMaskMetadata {
		// All test results have been masked to limited data only, so the test
		// metadata should be masked.
		tv.TestMetadata = nil
		tv.IsMasked = true
	}

	for _, exoneration := range tv.Exonerations {
		invID, _, _, err := pbutil.ParseTestExonerationName(exoneration.Name)
		if err != nil {
			return err
		}

		// Get the immediate invocation to which this test exoneration belongs.
		reachableInv, ok := q.ReachableInvocations.Invocations[invocations.ID(invID)]
		if !ok {
			return fmt.Errorf("error finding realm: invocation %s not found", invID)
		}

		// Check if the caller has permission to get test exonreations in the
		// immediate invocation's realm.
		allowedExoneration, ok := exonerationPerms[reachableInv.Realm]
		if !ok {
			// The test exoneration permission in this realm hasn't been checked
			// before.
			allowedExoneration, err = auth.HasPermission(ctx,
				rdbperms.PermGetTestExoneration, reachableInv.Realm, nil)
			if err != nil {
				return errors.Annotate(err,
					"error checking permission for test exoneration data restriction").Err()
			}
			// Record whether the caller has permission to get test exonerations in
			// the realm so the HasPermission result can be re-used.
			exonerationPerms[reachableInv.Realm] = allowedExoneration
		}

		if allowedExoneration {
			shouldMaskVariant = false
		} else {
			// Restrict test exoneration data.
			if err := exonerations.ToLimitedData(ctx, exoneration); err != nil {
				return err
			}
		}
	}

	if shouldMaskVariant {
		// All test results and test exonerations have been masked to limited data
		// only, so the variant definition should be masked.
		tv.Variant = nil
		tv.IsMasked = true
	}

	return nil
}

func (q *Query) queryTestVariantsWithUnexpectedResults(ctx context.Context, f func(*pb.TestVariant) error) (err error) {
	ctx, ts := tracing.Start(ctx, "testvariants.Query.run",
		attribute.Int("cr.dev.invocations", len(q.ReachableInvocations.Invocations)),
	)
	defer func() { tracing.End(ts, err) }()

	if q.PageSize < 0 {
		panic("PageSize < 0")
	}

	st, err := spanutil.GenerateStatement(testVariantsWithUnexpectedResultsSQLTmpl, map[string]any{
		"ResultColumns": strings.Join(q.resultSelectColumns(), ", "),
		"HasTestIds":    len(q.TestIDs) > 0,
		"StatusFilter":  q.Predicate.GetStatus() != 0 && q.Predicate.GetStatus() != pb.TestVariantStatus_UNEXPECTED_MASK,
	})
	if err != nil {
		return
	}
	st.Params = q.params
	st.Params["limit"] = q.PageSize
	st.Params["testResultLimit"] = q.ResultLimit

	var b spanutil.Buffer
	return spanutil.Query(ctx, st, func(row *spanner.Row) error {
		tv := &pb.TestVariant{}
		var tvStatus int64
		var results []*tvResult
		var exonerationIDs []string
		var exonerationInvocationIDs []string
		var exonerationExplanationHTMLs [][]byte
		var exonerationReasons []int64
		var tmd spanutil.Compressed
		err := b.FromSpanner(row,
			&tv.TestId, &tv.VariantHash, &tv.Variant, &tmd, &tvStatus, &results,
			&exonerationIDs, &exonerationInvocationIDs, &exonerationExplanationHTMLs,
			&exonerationReasons)
		if err != nil {
			return err
		}

		tv.Status = pb.TestVariantStatus(tvStatus)
		if tv.Status == pb.TestVariantStatus_EXPECTED {
			panic("query of test variants with unexpected results returned a test variant with only expected results.")
		}

		if err := populateTestMetadata(tv, tmd); err != nil {
			return errors.Annotate(err, "error unmarshalling test_metadata for %s", tv.TestId).Err()
		}

		// Populate tv.Results
		tv.Results = make([]*pb.TestResultBundle, len(results))
		for i, r := range results {
			tr, err := q.toTestResultProto(r, tv.TestId)
			if err != nil {
				return err
			}

			tv.Results[i] = &pb.TestResultBundle{
				Result: tr,
			}
		}

		// Populate tv.Exonerations
		if len(exonerationReasons) == 0 {
			return f(tv)
		}

		tv.Exonerations = make([]*pb.TestExoneration, len(exonerationReasons))
		for i, exonerationID := range exonerationIDs {
			// Due to query design, exonerationIDs, exonerationInvocationIDs,
			// exonerationExplanationHTMLs and exonerationReasons should all be
			// the same length.

			e := &pb.TestExoneration{}
			e.Name = pbutil.TestExonerationName(
				string(invocations.IDFromRowID(exonerationInvocationIDs[i])),
				tv.TestId, exonerationID)

			ex := exonerationExplanationHTMLs[i]
			if e.ExplanationHtml, err = q.decompressText(ex); err != nil {
				return err
			}
			e.Reason = pb.ExonerationReason(exonerationReasons[i])

			tv.Exonerations[i] = e
		}
		return f(tv)
	})
}

// Page represents a page of test variants fetched
// in response to a query.
type Page struct {
	// TestVariants are the test variants in the page.
	TestVariants []*pb.TestVariant
	// DistinctSources are the sources referenced by each test variant's
	// source_id, stored by their ID. The ID itself should be treated
	// as an opaque value; its generation is an implementation detail.
	DistinctSources map[string]*pb.Sources
	// NextPageToken is used to iterate to the next page of results.
	NextPageToken string
}

func (q *Query) fetchTestVariantsWithUnexpectedResults(ctx context.Context) (Page, error) {
	responseSize := 0
	tvs := make([]*pb.TestVariant, 0, q.PageSize)

	// resultPerms and exonerationPerms are maps in which:
	// * the key is a realm, and
	// * the value is whether the caller has permission in that realm to get
	//   either test results (resultPerms),
	//   or test exonerations (exonerationPerms).
	// They are populated by Query.toLimitedData as the test variants are fetched
	// so that each realm/permission combination is checked at most once.
	resultPerms := make(map[string]bool)
	exonerationPerms := make(map[string]bool)

	// Sources referenced from each test variant's source_id.
	distinctSources := make(map[string]*pb.Sources)

	// Fetch test variants with unexpected results.
	err := q.queryTestVariantsWithUnexpectedResults(ctx, func(tv *pb.TestVariant) error {
		// Populate the code sources tested.
		if err := q.populateSources(tv, distinctSources); err != nil {
			return errors.Annotate(err, "resolving sources").Err()
		}

		// Restrict test variant data as required.
		if q.AccessLevel != AccessLevelUnrestricted {
			if err := q.toLimitedData(ctx, tv, resultPerms, exonerationPerms); err != nil {
				return errors.Annotate(err, "applying limited access mask").Err()
			}
		}

		// Apply field mask.
		if err := q.trim(tv); err != nil {
			return errors.Annotate(err, "applying field mask").Err()
		}

		tvs = append(tvs, tv)

		// Apply soft repsonse size limit.
		responseSize += estimateTestVariantSize(tv)
		if responseSize > q.ResponseLimitBytes {
			return responseLimitReachedErr
		}
		return nil
	})
	if err != nil && err != responseLimitReachedErr {
		return Page{}, err
	}

	var nextPageToken string
	if len(tvs) == q.PageSize || err == responseLimitReachedErr {
		// There could be more test variants to return.
		last := tvs[len(tvs)-1]
		nextPageToken = pagination.Token(last.Status.String(), last.TestId, last.VariantHash)
	} else {
		// We have finished reading all test variants with unexpected results.
		if q.Predicate.GetStatus() != 0 {
			// The query is for test variants with specific status, so the query reaches
			// to its last results already.
			nextPageToken = ""
		} else {
			// If we got less than one page of test variants with unexpected results,
			// and the query is not for test variants with specific status,
			// compute the nextPageToken for test variants with only expected results.
			nextPageToken = pagination.Token(pb.TestVariantStatus_EXPECTED.String(), "", "")
		}
	}
	return Page{TestVariants: tvs, DistinctSources: distinctSources, NextPageToken: nextPageToken}, nil
}

// queryTestResults returns a page of test results, calling f for each
// test result read. Test results are returned in test variant order.
// Within each test variant, unexpected results are returned first.
func (q *Query) queryTestResults(ctx context.Context, limit int, f func(testId, variantHash string, variant *pb.Variant, tmd spanutil.Compressed, tvr *tvResult) error) (err error) {
	ctx, ts := tracing.Start(ctx, "testvariants.Query.queryTestResults",
		attribute.Int("cr.dev.invocations", len(q.ReachableInvocations.Invocations)),
	)
	defer func() { tracing.End(ts, err) }()
	st, err := spanutil.GenerateStatement(allTestResultsSQLTmpl, map[string]any{
		"ResultColumns": strings.Join(q.resultSelectColumns(), ", "),
		"HasTestIds":    len(q.TestIDs) > 0,
	})
	st.Params = q.params
	st.Params["limit"] = limit

	var b spanutil.Buffer
	return spanutil.Query(ctx, st, func(row *spanner.Row) error {
		var testID string
		var variantHash string
		variant := &pb.Variant{}
		var tmd spanutil.Compressed
		var results []*tvResult
		if err := b.FromSpanner(row, &testID, &variantHash, &variant, &tmd, &results); err != nil {
			return err
		}

		return f(testID, variantHash, variant, tmd, results[0])
	})
}

// queryTestVariants queries a page of test variants, calling f for each
// test variant read. Unless this method returns an error, it is guaranteed
// f will be called at least once, or finished will be set to true.
// finished indicates all test results from the invocation have been read.
//
// The number of test results provided per test variant is limited based
// on q.ResultLimit. Unexpected test results appear first in the
// list of test results provided on each test variant.
//
// The status of the test variant is not populated as exonerations are
// not read.
func (q *Query) queryTestVariants(ctx context.Context, f func(*pb.TestVariant) error) (finished bool, err error) {
	// Number of the total test results returned by the query.
	trCount := 0

	// The test variant we're processing right now.
	// It will be appended to tvs when all of its results are processed unless
	// it has unexpected results.
	var current *pb.TestVariant

	// Query q.PageSize+1 test results, so that in the case of all test results
	// correspond to one test variant, we will return q.PageSize test variants
	// instead of q.PageSize-1.
	pageSize := q.PageSize + 1

	// Ensure we always make forward progress, by having enough test results
	// to call the callback at least once.
	if q.ResultLimit > pageSize {
		pageSize = q.ResultLimit
	}

	// Number of test variants yielded so far. Used to ensure
	// we never yield more test variants than the page size.
	yieldCount := 0
	yield := func(tv *pb.TestVariant) error {
		yieldCount++
		// Never yield more test variants to the caller than the requested
		// query page size.
		if yieldCount <= q.PageSize {
			return f(tv)
		}
		return nil
	}

	err = q.queryTestResults(ctx, pageSize, func(testId, variantHash string, variant *pb.Variant, tmd spanutil.Compressed, tvr *tvResult) error {
		trCount++

		tr, err := q.toTestResultProto(tvr, testId)
		if err != nil {
			return err
		}

		var toYield *pb.TestVariant
		if current != nil {
			if current.TestId == testId && current.VariantHash == variantHash {
				// Another result within the same test variant.
				if len(current.Results) < q.ResultLimit {
					// Only append if within result limit.
					current.Results = append(current.Results, &pb.TestResultBundle{
						Result: tr,
					})
				}
				return nil
			}

			// Different TestId or VariantHash from current, so all test results of
			// current have been processed.
			toYield = current
		}

		// New test variant.
		current = &pb.TestVariant{
			TestId:      testId,
			VariantHash: variantHash,
			Variant:     variant,
			Results: []*pb.TestResultBundle{
				{
					Result: tr,
				},
			},
		}

		if err := populateTestMetadata(current, tmd); err != nil {
			return errors.Annotate(err, "error unmarshalling test_metadata for %s", current.TestId).Err()
		}
		if toYield != nil {
			return yield(toYield)
		}
		return nil
	})
	if err != nil {
		return false, err
	}

	if current != nil {
		// Yield current if:
		// - We finished reading all test results for the queried invocations, OR
		// - We have ResultLimit test results for the test variant.
		if len(current.Results) == q.ResultLimit || trCount < pageSize {
			err := yield(current)
			if err != nil {
				return false, err
			}
		}
	}

	// We have finished if we have run out of test results in the queried
	// invocations, and we managed to deliver all those test variants to
	// the caller.
	finished = trCount < pageSize && yieldCount <= q.PageSize
	return finished, nil
}

// responseLimitReachedErr is an error returned to avoid iterating over more
// results when the soft response size limit has been reached.
var responseLimitReachedErr = errors.New("terminating iteration early as response size limit reached")

func (q *Query) fetchTestVariantsWithOnlyExpectedResults(ctx context.Context) (Page, error) {
	responseSize := 0
	tvs := make([]*pb.TestVariant, 0, q.PageSize)

	// resultPerms and exonerationPerms are maps in which:
	// * the key is a realm, and
	// * the value is whether the caller has permission in that realm to get
	//   either test results (resultPerms),
	//   or test exonerations (exonerationPerms).
	// They are populated by Query.toLimitedData as the test variants are fetched so
	// that each realm/permission combination is checked at most once.
	resultPerms := make(map[string]bool)
	exonerationPerms := make(map[string]bool)

	// Sources referenced from each test variant's source_id.
	distinctSources := make(map[string]*pb.Sources)

	// The last test variant we have completely processed.
	var lastProcessedTestID string
	var lastProcessedVariantHash string

	finished, err := q.queryTestVariants(ctx, func(tv *pb.TestVariant) error {

		lastProcessedTestID = tv.TestId
		lastProcessedVariantHash = tv.VariantHash

		// Within the test variant, unexpected results appear first.
		// So if the first result is expected, all are.
		isOnlyExpected := tv.Results[0].Result.Expected
		if isOnlyExpected {
			tv.Status = pb.TestVariantStatus_EXPECTED

			// Populate the code sources tested.
			if err := q.populateSources(tv, distinctSources); err != nil {
				return errors.Annotate(err, "resolving sources").Err()
			}

			// Restrict test variant data as required.
			if q.AccessLevel != AccessLevelUnrestricted {
				if err := q.toLimitedData(ctx, tv, resultPerms, exonerationPerms); err != nil {
					return err
				}
			}

			// Apply field mask.
			if err := q.trim(tv); err != nil {
				return err
			}

			tvs = append(tvs, tv)

			// Apply soft repsonse size limit.
			responseSize += estimateTestVariantSize(tv)
			if responseSize > q.ResponseLimitBytes {
				return responseLimitReachedErr
			}
		}
		return nil
	})
	if err != nil && err != responseLimitReachedErr {
		return Page{}, err
	}

	var nextPageToken string
	if finished {
		nextPageToken = ""
	} else {
		nextPageToken = pagination.Token(pb.TestVariantStatus_EXPECTED.String(), lastProcessedTestID, lastProcessedVariantHash)
	}

	return Page{TestVariants: tvs, DistinctSources: distinctSources, NextPageToken: nextPageToken}, nil
}

// Fetch returns a page of test variants matching q.
// Returned test variants are ordered by test variant status, test ID and variant hash.
func (q *Query) Fetch(ctx context.Context) (Page, error) {
	if q.PageSize <= 0 {
		panic("PageSize <= 0")
	}
	if q.ResultLimit <= 0 {
		panic("ResultLimit <= 0")
	}
	if q.ResponseLimitBytes <= 0 {
		panic("ResponseLimitBytes <= 0")
	}

	status := int(q.Predicate.GetStatus())
	if q.Predicate.GetStatus() == pb.TestVariantStatus_UNEXPECTED_MASK {
		status = 0
	}
	testResultInvs, err := q.ReachableInvocations.WithTestResultsIDSet()
	if err != nil {
		return Page{}, errors.Annotate(err, "error getting invocations with test results").Err()
	}
	exonerationInvs, err := q.ReachableInvocations.WithExonerationsIDSet()
	if err != nil {
		return Page{}, errors.Annotate(err, "error getting invocations with exonerations").Err()
	}

	q.params = map[string]any{
		"testResultInvIDs":      testResultInvs,
		"testExonerationInvIDs": exonerationInvs,
		"testIDs":               q.TestIDs,
		"skipStatus":            int(pb.TestStatus_SKIP),
		"unexpected":            int(pb.TestVariantStatus_UNEXPECTED),
		"unexpectedlySkipped":   int(pb.TestVariantStatus_UNEXPECTEDLY_SKIPPED),
		"flaky":                 int(pb.TestVariantStatus_FLAKY),
		"exonerated":            int(pb.TestVariantStatus_EXONERATED),
		"expected":              int(pb.TestVariantStatus_EXPECTED),
		"status":                status,
	}

	var expected bool
	switch parts, err := pagination.ParseToken(q.PageToken); {
	case err != nil:
		return Page{}, err
	case len(parts) == 0:
		expected = false
		q.params["afterTvStatus"] = 0
		q.params["afterTestId"] = ""
		q.params["afterVariantHash"] = ""
	case len(parts) != 3:
		return Page{}, pagination.InvalidToken(errors.Reason("expected 3 components, got %q", parts).Err())
	default:
		status, ok := pb.TestVariantStatus_value[parts[0]]
		if !ok {
			return Page{}, pagination.InvalidToken(errors.Reason("unrecognized test variant status: %q", parts[0]).Err())
		}
		expected = pb.TestVariantStatus(status) == pb.TestVariantStatus_EXPECTED
		q.params["afterTvStatus"] = int(status)
		q.params["afterTestId"] = parts[1]
		q.params["afterVariantHash"] = parts[2]
	}

	if q.Predicate.GetStatus() == pb.TestVariantStatus_EXPECTED {
		expected = true
	}

	if expected {
		return q.fetchTestVariantsWithOnlyExpectedResults(ctx)
	} else {
		return q.fetchTestVariantsWithUnexpectedResults(ctx)
	}
}

var testVariantsWithUnexpectedResultsSQLTmpl = template.Must(template.New("testVariantsWithUnexpectedResultsSQL").Parse(`
	@{USE_ADDITIONAL_PARALLELISM=TRUE}
	WITH unexpectedTestVariants AS (
		SELECT DISTINCT TestId, VariantHash
		FROM TestResults@{FORCE_INDEX=UnexpectedTestResults, spanner_emulator.disable_query_null_filtered_index_check=true}
		WHERE IsUnexpected AND InvocationId in UNNEST(@testResultInvIDs)
	),

	-- Get test variants and their results.
	-- Also count the number of unexpected results and total results for each test
	-- variant, which will be used to classify test variants.
	test_variants AS (
		SELECT
			TestId,
			VariantHash,
			ANY_VALUE(Variant) Variant,
			ANY_VALUE(TestMetadata) TestMetadata,
			COUNTIF(IsUnexpected) num_unexpected,
			COUNTIF(Status=@skipStatus) num_skipped,
			COUNT(TestId) num_total,
			ARRAY_AGG(STRUCT({{.ResultColumns}})) results,
		FROM unexpectedTestVariants vur
		JOIN@{FORCE_JOIN_ORDER=TRUE, JOIN_METHOD=HASH_JOIN} TestResults tr USING (TestId, VariantHash)
		WHERE InvocationId in UNNEST(@testResultInvIDs)
		GROUP BY TestId, VariantHash
	),

	exonerated AS (
		SELECT
			TestId,
			VariantHash,
			ARRAY_AGG(ExonerationId) ExonerationIDs,
			ARRAY_AGG(InvocationId) InvocationIDs,
			ARRAY_AGG(ExplanationHTML) ExonerationExplanationHTMLs,
			ARRAY_AGG(Reason) ExonerationReasons
		FROM TestExonerations
		WHERE InvocationId IN UNNEST(@testExonerationInvIDs)
		GROUP BY TestId, VariantHash
	),

	testVariantsWithUnexpectedResults AS (
		SELECT
			tv.TestId,
			tv.VariantHash,
			tv.Variant,
			tv.TestMetadata,
			CASE
				WHEN exonerated.TestId IS NOT NULL THEN @exonerated
				WHEN num_unexpected = 0 THEN @expected -- should never happen in this query
				WHEN num_skipped = num_unexpected AND num_skipped = num_total THEN @unexpectedlySkipped
				WHEN num_unexpected = num_total THEN @unexpected
				ELSE @flaky
			END TvStatus,
			ARRAY(
				SELECT AS STRUCT *
				FROM UNNEST(tv.results)
				LIMIT @testResultLimit) results,
			exonerated.ExonerationIDs,
			exonerated.InvocationIDs,
			exonerated.ExonerationExplanationHTMLs,
			exonerated.ExonerationReasons
		FROM test_variants tv
		LEFT JOIN exonerated USING(TestId, VariantHash)
		ORDER BY TvStatus, TestId, VariantHash
	)

	SELECT
		TestId,
		VariantHash,
		Variant,
		TestMetadata,
		TvStatus,
		results,
		ExonerationIDs,
		InvocationIDs,
		ExonerationExplanationHTMLs,
		ExonerationReasons
	FROM testVariantsWithUnexpectedResults
	WHERE
	{{if .HasTestIds}}
		TestId in UNNEST(@testIDs) AND
	{{end}}
	{{if .StatusFilter}}
		(TvStatus = @status AND TestId > @afterTestId) OR
		(TvStatus = @status AND TestId = @afterTestId AND VariantHash > @afterVariantHash)
	{{else}}
		(TvStatus > @afterTvStatus) OR
		(TvStatus = @afterTvStatus AND TestId > @afterTestId) OR
		(TvStatus = @afterTvStatus AND TestId = @afterTestId AND VariantHash > @afterVariantHash)
	{{end}}
	ORDER BY TvStatus, TestId, VariantHash
	LIMIT @limit
`))

var allTestResultsSQLTmpl = template.Must(template.New("allTestResultsSQL").Parse(`
	@{USE_ADDITIONAL_PARALLELISM=TRUE}
	SELECT
		TestId,
		VariantHash,
		Variant,
		TestMetadata,

		-- Spanner doesn't support returning a struct as a column.
		-- https://cloud.google.com/spanner/docs/reference/standard-sql/query-syntax#using_structs_with_select
		-- Wrap it in an array instead.
		ARRAY(SELECT STRUCT({{.ResultColumns}})) AS results,
	FROM TestResults
	WHERE InvocationId in UNNEST(@testResultInvIDs)
	{{if .HasTestIds}}
		AND TestId in UNNEST(@testIDs)
	{{end}}
	AND (
		(TestId > @afterTestId) OR
		(TestId = @afterTestId AND VariantHash > @afterVariantHash)
	)
	-- Within a test variant, return the unexpected results first.
	ORDER BY TestId, VariantHash, IsUnexpected DESC
	LIMIT @limit
`))

func populateTestMetadata(tv *pb.TestVariant, tmd spanutil.Compressed) error {
	if len(tmd) == 0 {
		return nil
	}

	tv.TestMetadata = &pb.TestMetadata{}
	return proto.Unmarshal(tmd, tv.TestMetadata)
}

// estimateTestVariantSize estimates the size of a test variant in
// a pRPC response (pRPC responses use JSON serialisation).
func estimateTestVariantSize(tv *pb.TestVariant) int {
	// Estimate the size of a JSON-serialised test variant,
	// as the sum of the sizes of its fields, plus
	// an overhead (for JSON grammar and the field names).
	return 1000 + proto.Size(tv)
}
