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
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

// validateTestResult returns a non-nil error if msg is invalid.
func validateTestResult(now time.Time, msg *sinkpb.TestResult) (err error) {
	if msg == nil {
		return errors.Reason("unspecified").Err()
	}
	if err := pbutil.ValidateTestID(msg.TestId); err != nil {
		return errors.Annotate(err, "test_id").Err()
	}
	if err := pbutil.ValidateResultID(msg.ResultId); err != nil {
		return errors.Annotate(err, "result_id").Err()
	}
	if err := pbutil.ValidateTestResultStatus(msg.Status); err != nil {
		return errors.Annotate(err, "status").Err()
	}
	if err := pbutil.ValidateSummaryHTML(msg.SummaryHtml); err != nil {
		return errors.Annotate(err, "summary_html").Err()
	}
	if err := pbutil.ValidateStartTimeWithDuration(now, msg.StartTime, msg.Duration); err != nil {
		return err
	}
	if err := pbutil.ValidateStringPairs(msg.Tags); err != nil {
		return errors.Annotate(err, "tags").Err()
	}
	if err := validateArtifacts(msg.Artifacts); err != nil {
		return errors.Annotate(err, "artifacts").Err()
	}
	if msg.TestMetadata != nil {
		if err := pbutil.ValidateTestMetadata(toRdbTestMetadata(msg.TestMetadata)); err != nil {
			return errors.Annotate(err, "test_metadata").Err()
		}
	}
	if msg.FailureReason != nil {
		if err := pbutil.ValidateFailureReason(msg.FailureReason); err != nil {
			return errors.Annotate(err, "failure_reason").Err()
		}
	}
	if msg.Properties != nil {
		if err := pbutil.ValidateTestResultProperties(msg.Properties); err != nil {
			return errors.Annotate(err, "properties").Err()
		}
	}
	return nil
}

// validateArtifact returns a non-nil error if art is invalid.
func validateArtifact(art *sinkpb.Artifact) error {
	if art.GetFilePath() == "" && art.GetContents() == nil && art.GetGcsUri() == "" {
		return errors.Reason("body: one of file_path or contents or gcs_uri must be provided").Err()
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
			return errors.Reason("%s: unspecified", id).Err()
		}
		if err := pbutil.ValidateArtifactID(id); err != nil {
			return errors.Annotate(err, "%s", id).Err()
		}
		if err := validateArtifact(art); err != nil {
			return errors.Annotate(err, "%s", id).Err()
		}
	}
	return nil
}
