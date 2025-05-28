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

package testresults

import (
	"context"
	"net/url"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// MustParseName retrieves the invocation ID, unescaped test id, and
// result ID.
//
// Panics if the name is invalid. Should be used only with trusted data.
//
// MustParseName is faster than pbutil.ParseTestResultName.
func MustParseName(name string) (invID invocations.ID, testID, resultID string) {
	parts := strings.Split(name, "/")
	if len(parts) != 6 || parts[0] != "invocations" || parts[2] != "tests" || parts[4] != "results" {
		panic(errors.Fmt("malformed test result name: %q", name))
	}

	invID = invocations.ID(parts[1])
	testID = parts[3]
	resultID = parts[5]

	unescaped, err := url.PathUnescape(testID)
	if err != nil {
		panic(errors.Fmt("malformed test id %q: %w", testID, err))
	}
	testID = unescaped

	return
}

// Read reads specified TestResult within the transaction.
// If the TestResult does not exist, the returned error is annotated with
// NotFound GRPC code.
func Read(ctx context.Context, name string) (*pb.TestResult, error) {
	invID, testID, resultID := MustParseName(name)
	tr := &pb.TestResult{
		Name:     name,
		TestId:   testID,
		ResultId: resultID,
		Expected: true,
	}
	var (
		maybeUnexpected     spanner.NullBool
		micros              spanner.NullInt64
		summaryHTML         spanutil.Compressed
		tmd                 spanutil.Compressed
		fr                  spanutil.Compressed
		properties          spanutil.Compressed
		skipReason          spanner.NullInt64
		skippedReason       spanutil.Compressed
		frameworkExtensions spanutil.Compressed
	)
	err := spanutil.ReadRow(ctx, "TestResults", invID.Key(testID, resultID), map[string]any{
		"Variant":             &tr.Variant,
		"VariantHash":         &tr.VariantHash,
		"IsUnexpected":        &maybeUnexpected,
		"Status":              &tr.Status,
		"StatusV2":            &tr.StatusV2,
		"SummaryHTML":         &summaryHTML,
		"StartTime":           &tr.StartTime,
		"RunDurationUsec":     &micros,
		"Tags":                &tr.Tags,
		"TestMetadata":        &tmd,
		"FailureReason":       &fr,
		"Properties":          &properties,
		"SkipReason":          &skipReason,
		"SkippedReason":       &skippedReason,
		"FrameworkExtensions": &frameworkExtensions,
	})
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return nil, appstatus.Attachf(err, codes.NotFound, "%s not found", name)

	case err != nil:
		return nil, errors.Fmt("fetch %q: %w", name, err)
	}
	// Populate structured test ID from flat-form ID and variant.
	tr.TestIdStructured, err = pbutil.ParseStructuredTestIdentifierForOutput(testID, tr.Variant)
	if err != nil {
		return nil, errors.Fmt("parse structured test identifier: %w", err)
	}

	tr.SummaryHtml = string(summaryHTML)
	PopulateExpectedField(tr, maybeUnexpected)
	PopulateDurationField(tr, micros)
	PopulateSkipReasonField(tr, skipReason)
	if err := PopulateTestMetadata(tr, tmd); err != nil {
		return nil, errors.Fmt("unmarshal test metadata: %w", err)
	}
	if err := PopulateFailureReason(tr, fr); err != nil {
		return nil, errors.Fmt("unmarshal failure reason: %w", err)
	}
	if err := PopulateProperties(tr, properties); err != nil {
		return nil, errors.Fmt("unmarshal properties: %w", err)
	}
	if err := PopulateSkippedReason(tr, skippedReason); err != nil {
		return nil, errors.Fmt("unmarshal skipped reason: %w", err)
	}
	if err := PopulateFrameworkExtensions(tr, frameworkExtensions); err != nil {
		return nil, errors.Fmt("unmarshal framework extensions: %w", err)
	}
	return tr, nil
}

// PopulateDurationField populates tr.Duration from NullInt64.
func PopulateDurationField(tr *pb.TestResult, micros spanner.NullInt64) {
	tr.Duration = nil
	if micros.Valid {
		tr.Duration = durationpb.New(time.Duration(1000 * micros.Int64))
	}
}

// PopulateSkipReasonField populates tr.SkipReason from NullInt64.
func PopulateSkipReasonField(tr *pb.TestResult, skipReason spanner.NullInt64) {
	tr.SkipReason = pb.SkipReason_SKIP_REASON_UNSPECIFIED
	if skipReason.Valid {
		tr.SkipReason = pb.SkipReason(skipReason.Int64)
	}
}

// PopulateExpectedField populates tr.Expected from NullBool.
func PopulateExpectedField(tr *pb.TestResult, maybeUnexpected spanner.NullBool) {
	tr.Expected = !maybeUnexpected.Valid || !maybeUnexpected.Bool
}

func PopulateTestMetadata(tr *pb.TestResult, tmd spanutil.Compressed) error {
	if len(tmd) == 0 {
		return nil
	}

	tr.TestMetadata = &pb.TestMetadata{}
	return proto.Unmarshal(tmd, tr.TestMetadata)
}

// PopulateFailureReason converts a stored failure reason into its external
// proto representation.
// This should be called on all read paths from Spanner.
func PopulateFailureReason(tr *pb.TestResult, fr spanutil.Compressed) error {
	if len(fr) == 0 {
		tr.FailureReason = nil
		return nil
	}

	result := &pb.FailureReason{}
	err := proto.Unmarshal(fr, result)
	if err != nil {
		return err
	}

	// TODO: This call can safely be removed from November 2026 onwards
	// (after all data inserted prior to May 2025 has been deleted).
	// It is necessary to handle data uploaded prior to May 2025 which
	// was not always stored in normalised form.
	NormaliseFailureReason(result)

	// Populate output only fields.
	PopulateFailureReasonOutputOnlyFields(result)

	tr.FailureReason = result
	return nil
}

// NormaliseFailureReason handles compatibility of legacy failure reason uploads,
// converting them to a normalised failure reason representation for storage.
// This also depopulates any OUTPUT_ONLY fields.
//
// This should be called before storing the results or
// PopulateFailureReasonOutputOnlyFields.
func NormaliseFailureReason(fr *pb.FailureReason) {
	if len(fr.Errors) == 0 && fr.PrimaryErrorMessage != "" {
		// Older results: normalise by set Errors collection from
		// PrimaryErrorMessage.
		fr.Errors = []*pb.FailureReason_Error{{Message: fr.PrimaryErrorMessage}}
	}
	// TODO(meiring): Uncomment this line once we are confident
	// we will not roll back the original change. We need to
	// keep storing PrimaryErrorMessage for a short time lest
	// we roll back to a version of ResultDB that returns
	// read failure reasons verbatim (without computing this field).
	// fr.PrimaryErrorMessage = ""
}

// PopulateFailureReasonOutputOnlyFields populates output only fields
// for a normalised test result.
func PopulateFailureReasonOutputOnlyFields(fr *pb.FailureReason) {
	if len(fr.Errors) > 0 {
		// Ppulate PrimaryErrorMessage from Errors collection.
		fr.PrimaryErrorMessage = fr.Errors[0].Message
	} else {
		fr.PrimaryErrorMessage = ""
	}
}

func PopulateProperties(tr *pb.TestResult, properties spanutil.Compressed) error {
	if len(properties) == 0 {
		return nil
	}

	tr.Properties = &structpb.Struct{}
	return proto.Unmarshal(properties, tr.Properties)
}

// PopulateSkippedReason populates the skipped reason field
// from its stored representation.
// This should be called on all read paths from Spanner.
func PopulateSkippedReason(tr *pb.TestResult, sr spanutil.Compressed) error {
	if len(sr) == 0 {
		tr.SkippedReason = nil
		return nil
	}

	result := &pb.SkippedReason{}
	err := proto.Unmarshal(sr, result)
	if err != nil {
		return err
	}

	tr.SkippedReason = result
	return nil
}

// PopulateFrameworkExtensions populates the framework extension field
// from its stored representation.
// This should be called on all read paths from Spanner.
func PopulateFrameworkExtensions(tr *pb.TestResult, sr spanutil.Compressed) error {
	if len(sr) == 0 {
		tr.FrameworkExtensions = nil
		return nil
	}

	result := &pb.FrameworkExtensions{}
	err := proto.Unmarshal(sr, result)
	if err != nil {
		return err
	}

	tr.FrameworkExtensions = result
	return nil
}
