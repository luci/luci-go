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
	"regexp"

	"go.chromium.org/luci/common/errors"

	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
)

const (
	// The regex rule that all artifact name must conform to.
	artifactNameRegex = `^[[:word:]]([[:print:]]*[[:word:]])?$`
	testIDRegex       = artifactNameRegex
	resultIDRegex     = testIDRegex
)

func validateStr(r, fv, fn string) error {
	re := regexp.MustCompile(r)
	if !re.MatchString(fv) {
		return errors.Reason("invalid %s - %q", fn, fv).Err()
	}
	return nil
}

func validateUploadTestResult(msg *sinkpb.TestResult) error {
	if err := validateStr(testIDRegex, msg.TestId, "TestID"); err != nil {
		return err
	}
	if err := validateStr(resultIDRegex, msg.ResultId, "ResultID"); err != nil {
		return err
	}

	// check all the input artifact files are accessible.
	var errs errors.MultiError
	for n, ar := range msg.InputArtifacts {
		if err := validateArtifact(n, ar); err != nil {
			errs = append(errs, err)
		}
	}
	for n, ar := range msg.OutputArtifacts {
		if err := validateArtifact(n, ar); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errs
	}

	return nil
}

func validateUploadTestResultFile(msg *sinkpb.TestResultFile) error {
	if err := checkFileAccess(msg.Path); err != nil {
		return err
	}
	return nil
}

func validateArtifact(n string, ar *sinkpb.Artifact) error {
	if err := validateStr(artifactNameRegex, n, "artifact name"); err != nil {
		return err
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
	// Is an empty file a valid TestResultFile or Artifact?
	// TODO(crbug/1017288) - check the maximum test_result file size
	return nil
}
