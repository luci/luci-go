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

package datastoreutil

import (
	"context"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/model"
)

// GetTestFailures returns all TestFailures for a test.
// Optionally filter by variant_hash.
func GetTestFailures(c context.Context, project, testID, refHash, variantHash string) ([]*model.TestFailure, error) {
	testFailures := []*model.TestFailure{}
	q := datastore.NewQuery("TestFailure").
		Eq("project", project).
		Eq("test_id", testID).
		Eq("ref_hash", refHash)
	if variantHash != "" {
		q = q.Eq("variant_hash", variantHash)
	}
	if err := datastore.GetAll(c, q, &testFailures); err != nil {
		return nil, err
	}
	return testFailures, nil
}
