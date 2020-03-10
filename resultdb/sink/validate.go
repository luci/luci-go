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

// Package sink provides a server for aggregating test results and sending them
// to the ResultDB backend.
package sink

import (
	"os"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/resultdb/pbutil"

	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
)

func validateUploadTestResult(msg *sinkpb.TestResult) error {
	if err := pbutil.ValidateTestID(msg.TestId); err != nil {
		return errors.Annotate(err, "test_id %q", msg.TestId).Err()
	}
	if err := pbutil.ValidateResultID(msg.ResultId); err != nil {
		return errors.Annotate(err, "result_id %q", msg.ResultId).Err()
	}

	// check the input and output artifact files are accessible.
	for n, ar := range msg.InputArtifacts {
		if err := validateArtifact(n, ar); err != nil {
			return err
		}
	}
	for n, ar := range msg.OutputArtifacts {
		if err := validateArtifact(n, ar); err != nil {
			return err
		}
	}
	return nil
}

func validateUploadTestResultFile(msg *sinkpb.TestResultFile) error {
	return checkFileAccess(msg.Path); err != nil {
}

func validateArtifact(n string, ar *sinkpb.Artifact) error {
	if err := pbutil.ValidateArtifactName(n); err != nil {
		return errors.Annotate(err, "artifact.name %q", n).Err()
	}
	if p := ar.GetFilePath(); p != "" {
		if err := checkFileAccess(p); err != nil {
			return err
		}
	}
	return nil
}

// checkFileAccess returns nil if a given file is a valid, readable file, or
// an error with the reason, otherwise.
func checkFileAccess(p string) error {
	info, err := os.Stat(p)
	if err != nil {
		return err
	}
	m := info.Mode()
	if !m.IsRegular() {
		return errors.Reason("%q is not a regular file, but %d", p, m).Err()
	}

	f, err := os.OpenFile(p, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	// TODO(crbug/1017288) - check the maximum test_result file size
	return nil
}
