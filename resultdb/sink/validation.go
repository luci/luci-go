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

package sink

import (
	"mime"
	"time"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/pbutil"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

// validateTestResult returns a non-nil error if the result sink result is invalid.
// This performs basic validation agnostic of the resultsink server configuration,
// a second validation pass will be performed on the result merged with test prefixes,
// etc. etc. configured on the server.
//
// Note: This will not pick up all errors on results uploaded to ResultDB,
// but it will pick up a significant set.
func validateTestResult(now time.Time, msg *sinkpb.TestResult, usingStructuredID bool) (err error) {
	if msg == nil {
		return errors.New("unspecified")
	}

	// If the flat test ID field is present, validate it.
	// Note: Some clients report empty test ID (for suites that have only one result),
	// expecting a valid test ID to come from concatenation with a test prefix.
	if msg.TestId != "" {
		if err := pbutil.ValidateTestID(msg.TestId); err != nil {
			return errors.Fmt("test_id: %w", err)
		}
	}
	// If structured test ID is present, validate it.
	if msg.TestIdStructured != nil || usingStructuredID {
		if msg.TestIdStructured == nil {
			return errors.New("test_id_structured: unspecified")
		}
		if len(msg.TestIdStructured.CaseNameComponents) == 0 {
			return errors.New("test_id_structured: case_name_components: unspecified")
		}
		// Perform basic validation, using a placeholder module name and scheme.
		baseID := pbutil.BaseTestIdentifier{
			ModuleName:   "placeholder",
			ModuleScheme: "scheme",
			CoarseName:   msg.TestIdStructured.CoarseName,
			FineName:     msg.TestIdStructured.FineName,
			CaseName:     pbutil.EncodeCaseName(msg.TestIdStructured.CaseNameComponents...),
		}

		if err := pbutil.ValidateBaseTestIdentifier(baseID); err != nil {
			return errors.Fmt("test_id_structured: %w", err)
		}
	}

	if err := pbutil.ValidateResultID(msg.ResultId); err != nil {
		return errors.Fmt("result_id: %w", err)
	}
	// This performs lightweight validation of status fields. A more comprehensive
	// validation is performed by ResultSink later using pbutil.ValidateTestResult.
	if msg.Status == resultpb.TestStatus_STATUS_UNSPECIFIED || msg.StatusV2 != resultpb.TestResult_STATUS_UNSPECIFIED {
		if err := pbutil.ValidateTestResultStatusV2(msg.StatusV2); err != nil {
			return errors.Fmt("status_v2: %w", err)
		}
	}
	if msg.Status != resultpb.TestStatus_STATUS_UNSPECIFIED {
		if err := pbutil.ValidateTestResultStatus(msg.Status); err != nil {
			return errors.Fmt("status: %w", err)
		}
	}
	if err := pbutil.ValidateSummaryHTML(msg.SummaryHtml); err != nil {
		return errors.Fmt("summary_html: %w", err)
	}
	if err := pbutil.ValidateStartTimeWithDuration(now, msg.StartTime, msg.Duration); err != nil {
		return err
	}
	if err := pbutil.ValidateStringPairs(msg.Tags); err != nil {
		return errors.Fmt("tags: %w", err)
	}
	if err := validateArtifacts(msg.Artifacts); err != nil {
		return errors.Fmt("artifacts: %w", err)
	}
	if msg.TestMetadata != nil {
		if err := pbutil.ValidateTestMetadata(msg.TestMetadata); err != nil {
			return errors.Fmt("test_metadata: %w", err)
		}
	}
	if msg.FailureReason != nil {
		useStrictValidation := msg.StatusV2 != resultpb.TestResult_STATUS_UNSPECIFIED
		if err := pbutil.ValidateFailureReason(msg.FailureReason, useStrictValidation); err != nil {
			return errors.Fmt("failure_reason: %w", err)
		}
	}
	if msg.Properties != nil {
		if err := pbutil.ValidateTestResultProperties(msg.Properties); err != nil {
			return errors.Fmt("properties: %w", err)
		}
	}
	if msg.SkippedReason != nil {
		if err := pbutil.ValidateSkippedReason(msg.SkippedReason); err != nil {
			return errors.Fmt("skipped_reason: %w", err)
		}
	}
	if msg.FrameworkExtensions != nil {
		if err := pbutil.ValidateFrameworkExtensions(msg.FrameworkExtensions, msg.StatusV2); err != nil {
			return errors.Fmt("framework_extensions: %w", err)
		}
	}
	return nil
}

// validateArtifact returns a non-nil error if art is invalid.
func validateArtifact(art *sinkpb.Artifact) error {
	if art.GetFilePath() == "" && art.GetContents() == nil && art.GetGcsUri() == "" {
		return errors.New("body: one of file_path or contents or gcs_uri must be provided")
	}
	if art.GetContentType() != "" {
		_, _, err := mime.ParseMediaType(art.ContentType)
		if err != nil {
			return err
		}
	}
	return nil
}

// validateArtifacts returns a non-nil error if any element of arts is invalid.
func validateArtifacts(arts map[string]*sinkpb.Artifact) error {
	for id, art := range arts {
		if art == nil {
			return errors.Fmt("%s: unspecified", id)
		}
		if err := pbutil.ValidateArtifactID(id); err != nil {
			return errors.Fmt("%s: %w", id, err)
		}
		if err := validateArtifact(art); err != nil {
			return errors.Fmt("%s: %w", id, err)
		}
	}
	return nil
}
