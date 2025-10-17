// Copyright 2025 The LUCI Authors.
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

package testrealms

import (
	"context"
	"slices"
	"strings"

	"go.chromium.org/luci/analysis/internal/pagination"
)

type TestRealm struct {
	TestID   string
	TestName string
	Realm    string
}

// Provides a fake implementation of the test search client for testing purposes.
type FakeClient struct {
	// TestRealms to search over.
	TestRealms []TestRealm
}

func (c *FakeClient) QueryTests(ctx context.Context, project, testIDSubstring string, opts QueryTestsOptions) (testIDs []string, nextPageToken string, err error) {
	paginationTestID := ""
	if opts.PageToken != "" {
		paginationTestID, err = parseQueryTestsPageToken(opts.PageToken)
		if err != nil {
			return nil, "", err
		}
	}

	// Ensure TestRealms are sorted.
	slices.SortFunc(c.TestRealms, func(a, b TestRealm) int {
		return strings.Compare(a.TestID, b.TestID)
	})

	for _, tr := range c.TestRealms {
		if paginationTestID != "" && tr.TestID <= paginationTestID {
			continue
		}
		if !slices.Contains(opts.Realms, tr.Realm) {
			continue
		}
		if opts.CaseSensitive {
			if strings.Contains(tr.TestID, testIDSubstring) || strings.Contains(tr.TestName, testIDSubstring) {
				testIDs = append(testIDs, tr.TestID)
			}
		} else {
			if strings.Contains(strings.ToLower(tr.TestID), strings.ToLower(testIDSubstring)) || strings.Contains(strings.ToLower(tr.TestName), strings.ToLower(testIDSubstring)) {
				testIDs = append(testIDs, tr.TestID)
			}
		}
		if len(testIDs) == opts.PageSize {
			break
		}
	}

	if opts.PageSize != 0 && len(testIDs) == opts.PageSize {
		lastTestID := testIDs[len(testIDs)-1]
		nextPageToken = pagination.Token(lastTestID)
	}
	return testIDs, nextPageToken, nil
}
