// Copyright 2026 The LUCI Authors.
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

package analysis

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"

	analysispb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/client/cmd/luci/ids"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTestHistorySubstringSearch(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 10, 9, 2, 0, 0, 0, time.UTC)

	ftt.Run("TestHistoryCmd supports -test-substring discovery via TestHistory.QueryTests", t, func(t *ftt.Test) {
		t.Run("Auto-resolves unique -test-substring match via TestHistory.QueryTests", func(t *ftt.Test) {
			const canonicalID = `://chrome/test\:browser_tests!gtest::WebAppBrowserTest#BrowserDisplayNotInstallable/All.0`
			var capturedQueryReq *analysispb.QueryTestsRequest
			var statsTestID string
			thMock := &mockTestHistoryClient{
				queryTests: func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
					capturedQueryReq = in
					return &analysispb.QueryTestsResponse{
						TestIds: []string{canonicalID},
					}, nil
				},
				queryStats: func(ctx context.Context, in *analysispb.QueryTestHistoryStatsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryStatsResponse, error) {
					statsTestID = in.TestId
					assert.Loosely(t, in.Predicate.GetSubRealm(), should.Equal("ci"))
					return &analysispb.QueryTestHistoryStatsResponse{
						Groups: []*analysispb.QueryTestHistoryStatsResponse_Group{
							{
								VariantHash:   "hash-linux",
								VerdictCounts: &analysispb.QueryTestHistoryStatsResponse_Group_VerdictCounts{Failed: 2, Passed: 8},
							},
						},
					}, nil
				},
				query: func(ctx context.Context, in *analysispb.QueryTestHistoryRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryResponse, error) {
					return &analysispb.QueryTestHistoryResponse{
						Verdicts: []*analysispb.TestVerdict{
							{
								TestId:       in.TestId,
								VariantHash:  "hash-linux",
								InvocationId: "build-87654321",
								StatusV2:     analysispb.TestVerdict_FAILED,
							},
						},
					}, nil
				},
			}

			code, out, errOut := execTestHistoryCmd(
				thMock,
				fixedNow,
				"-project", "chromium",
				"-subrealm", "chromium:ci",
				"-test-substring", "WebAppBrowserTest#BrowserDisplayNotInstallable",
				"-days", "14",
			)
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, errOut, should.BeEmpty)
			assert.Loosely(t, capturedQueryReq.GetProject(), should.Equal("chromium"))
			assert.Loosely(t, capturedQueryReq.GetSubRealm(), should.Equal("ci"))
			assert.Loosely(t, capturedQueryReq.GetTestIdSubstring(), should.Equal("WebAppBrowserTest#BrowserDisplayNotInstallable"))
			assert.Loosely(t, capturedQueryReq.GetPageSize(), should.Equal(int32(testHistoryPageSize)))
			assert.Loosely(t, capturedQueryReq.GetCaseInsensitive(), should.BeFalse)
			assert.Loosely(t, statsTestID, should.Equal(canonicalID))
			assert.Loosely(t, out, should.ContainSubstring("Resolved:  \"WebAppBrowserTest#BrowserDisplayNotInstallable\" -> "+canonicalID+"\n"))
			assert.Loosely(t, out, should.ContainSubstring("Module:    //chrome/test:browser_tests (scheme: gtest)"))
			assert.Loosely(t, out, should.ContainSubstring("- FAILED build-87654321"))
		})

		t.Run("Disambiguates multiple -test-substring matches without fetching history", func(t *ftt.Test) {
			const browserTestsID = `://chrome/test\:browser_tests!gtest::WebAppBrowserTest#MyCase`
			const interactiveTestsID = `://chrome/test\:interactive_ui_tests!gtest::WebAppBrowserTest#MyCase`
			const legacyBrowserTestsID = "ninja://chrome/test:browser_tests/WebAppBrowserTest.MyCase"

			t.Run("Ranks structured IDs ahead of legacy IDs, deduplicates, and preserves filter flags in Next step", func(t *ftt.Test) {
				statsCalled := false
				thMock := &mockTestHistoryClient{
					queryTests: func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
						return &analysispb.QueryTestsResponse{
							TestIds: []string{"", legacyBrowserTestsID, interactiveTestsID, browserTestsID, browserTestsID},
						}, nil
					},
					queryStats: func(ctx context.Context, in *analysispb.QueryTestHistoryStatsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryStatsResponse, error) {
						statsCalled = true
						return &analysispb.QueryTestHistoryStatsResponse{}, nil
					},
				}

				code, out, errOut := execTestHistoryCmd(
					thMock,
					fixedNow,
					"-project", "chromium",
					"-subrealm", "ci",
					"-variant", "builder:linux-rel",
					"-days", "21",
					"-limit", "5",
					"-test-substring", "WebAppBrowserTest",
				)
				assert.Loosely(t, code, should.Equal(0))
				assert.Loosely(t, errOut, should.BeEmpty)
				assert.Loosely(t, statsCalled, should.BeFalse)
				assert.Loosely(t, out, should.ContainSubstring("Multiple matching test IDs found (3); specify exact -testid to view history:"))

				expectedList := "- " + browserTestsID + "\n" +
					"- " + interactiveTestsID + "\n" +
					"- " + legacyBrowserTestsID + "\n"
				assert.Loosely(t, out, should.ContainSubstring(expectedList))
				assert.Loosely(t, out, should.ContainSubstring(
					"luci analysis test-history -project chromium -subrealm ci -variant builder:linux-rel -days 21 -limit 5 -testid '"+browserTestsID+"'"))
			})

			t.Run("Does not auto-resolve page-truncated responses and caps disambiguation output at maxMatchingTestsShown", func(t *ftt.Test) {
				thMock := &mockTestHistoryClient{
					queryTests: func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
						return &analysispb.QueryTestsResponse{
							TestIds:       []string{browserTestsID},
							NextPageToken: "next-page",
						}, nil
					},
				}
				code, out, errOut := execTestHistoryCmd(thMock, fixedNow, "-project", "chromium", "-test-substring", "WebAppBrowserTest#MyCase")
				assert.Loosely(t, code, should.Equal(0))
				assert.Loosely(t, errOut, should.BeEmpty)
				assert.Loosely(t, out, should.ContainSubstring("Multiple matching test IDs found (1+, page cap reached); specify exact -testid to view history:"))

				many := make([]string, 23)
				for i := range many {
					many[i] = fmt.Sprintf("://mod\\:tests!gtest::Suite#Case%02d", i)
				}
				var disambigBuf bytes.Buffer
				printSubstringDisambiguation(&disambigBuf, "chromium", "", "Case", many, false, nil, false, 7, false, 10)
				assert.Loosely(t, disambigBuf.String(), should.ContainSubstring("... +3 more matching test IDs"))
				assert.Loosely(t, disambigBuf.String(), should.ContainSubstring(
					"luci analysis test-history -project chromium -testid '://mod\\:tests!gtest::Suite#Case00'\n"))
			})
		})

		t.Run("Supports -case-insensitive, automatic case-insensitive retry, RPC errors, and zero-match output", func(t *ftt.Test) {
			const structuredID = `://chrome/test\:browser_tests!gtest::WebAppBrowserTest#MyCase`

			var calls []bool
			thMock := &mockTestHistoryClient{
				queryTests: func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
					calls = append(calls, in.GetCaseInsensitive())
					if !in.GetCaseInsensitive() {
						return &analysispb.QueryTestsResponse{}, nil
					}
					return &analysispb.QueryTestsResponse{
						TestIds: []string{structuredID},
					}, nil
				},
			}

			code, out, errOut := execTestHistoryCmd(thMock, fixedNow, "-project", "chromium", "-test-substring", "webappbrowsertest#mycase")
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, errOut, should.BeEmpty)
			assert.Loosely(t, calls, should.Match([]bool{false, true}))
			assert.Loosely(t, out, should.ContainSubstring(`Resolved:  "webappbrowsertest#mycase" -> `+structuredID))

			// Explicit -case-insensitive skips the exact-case call
			calls = nil
			code, _, errOut = execTestHistoryCmd(thMock, fixedNow, "-project", "chromium", "-test-substring", "webappbrowsertest#mycase", "-case-insensitive")
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, errOut, should.BeEmpty)
			assert.Loosely(t, calls, should.Match([]bool{true}))

			// RPC failure on primary call or automatic case-insensitive retry exits with code 1
			errPrimaryMock := &mockTestHistoryClient{
				queryTests: func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
					return nil, errors.New("bq unavailable")
				},
			}
			code, _, errOut = execTestHistoryCmd(errPrimaryMock, fixedNow, "-project", "chromium", "-test-substring", "MyCase")
			assert.Loosely(t, code, should.Equal(1))
			assert.Loosely(t, errOut, should.ContainSubstring("TestHistory.QueryTests RPC failed"))

			errRetryMock := &mockTestHistoryClient{
				queryTests: func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
					if !in.GetCaseInsensitive() {
						return &analysispb.QueryTestsResponse{}, nil
					}
					return nil, errors.New("deadline exceeded on retry")
				},
			}
			code, _, errOut = execTestHistoryCmd(errRetryMock, fixedNow, "-project", "chromium", "-test-substring", "MyCase")
			assert.Loosely(t, code, should.Equal(1))
			assert.Loosely(t, errOut, should.ContainSubstring("deadline exceeded on retry"))

			// Zero matches after retry prints clear 90-day empty message with exit code 0
			emptyMock := &mockTestHistoryClient{}
			code, out, errOut = execTestHistoryCmd(emptyMock, fixedNow, "-project", "chromium", "-test-substring", "NonExistentTestName")
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, errOut, should.BeEmpty)
			assert.Loosely(t, out, should.ContainSubstring("No matching test IDs found in the last 90 days."))
		})

		t.Run("Validates -test-substring flag combinations, length limits, and positional project target", func(t *ftt.Test) {
			thMock := &mockTestHistoryClient{}

			code, _, errOut := execTestHistoryCmd(thMock, fixedNow, "-project", "chromium", "-testid", "foo", "-test-substring", "bar")
			assert.Loosely(t, code, should.Equal(1))
			assert.Loosely(t, errOut, should.ContainSubstring("specify either -test-substring or -testid, not both"))

			// Explicit empty -test-substring fails validation and enforces mutual exclusion
			code, _, errOut = execTestHistoryCmd(thMock, fixedNow, "-project", "chromium", "-test-substring", "   ")
			assert.Loosely(t, code, should.Equal(1))
			assert.Loosely(t, errOut, should.ContainSubstring("-test-substring cannot be empty"))

			code, _, errOut = execTestHistoryCmd(thMock, fixedNow, "-project", "chromium", "-testid", "foo", "-test-substring", "")
			assert.Loosely(t, code, should.Equal(1))
			assert.Loosely(t, errOut, should.ContainSubstring("specify either -test-substring or -testid, not both"))

			code, _, errOut = execTestHistoryCmd(thMock, fixedNow, "-project", "chromium", "-testid", "", "-test-substring", "bar")
			assert.Loosely(t, code, should.Equal(1))
			assert.Loosely(t, errOut, should.ContainSubstring("specify either -test-substring or -testid, not both"))

			code, _, errOut = execTestHistoryCmd(thMock, fixedNow, "-project", "chromium", "-test-substring", "bad\x00char")
			assert.Loosely(t, code, should.Equal(1))
			assert.Loosely(t, errOut, should.ContainSubstring("invalid -test-substring"))

			code, _, errOut = execTestHistoryCmd(thMock, fixedNow, "-project", "chromium", "-testid", "foo", "-case-insensitive")
			assert.Loosely(t, code, should.Equal(1))
			assert.Loosely(t, errOut, should.ContainSubstring("-case-insensitive requires -test-substring"))

			code, _, errOut = execTestHistoryCmd(thMock, fixedNow, "-project", "chromium", "-modulename", "//m", "-scheme", "gtest", "-case", "c", "-test-substring", "bar")
			assert.Loosely(t, code, should.Equal(1))
			assert.Loosely(t, errOut, should.ContainSubstring("specify either -test-substring or structured test ID flags"))

			miloURL := "https://ci.chromium.org/ui/test/turquoise/fuchsia-pkg%3A%2F%2Ffuchsia.com%2Fnetstack%23meta%2Ftest.cm"
			code, _, errOut = execTestHistoryCmd(thMock, fixedNow, miloURL, "-test-substring", "netstack")
			assert.Loosely(t, code, should.Equal(1))
			assert.Loosely(t, errOut, should.ContainSubstring("specify either -test-substring or a positional target containing a test ID, not both"))

			code, _, errOut = execTestHistoryCmd(thMock, fixedNow, "-project", "chromium", "-test-substring", strings.Repeat("a", maxTestSubstringLength+1))
			assert.Loosely(t, code, should.Equal(1))
			assert.Loosely(t, errOut, should.ContainSubstring("longer than 512 bytes"))

			// Positional rule URL supplying -project (without a test ID) works alongside -test-substring
			const resolvedID = `://chrome/test\:browser_tests!gtest::WebAppBrowserTest#MyCase`
			thMock.queryTests = func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
				assert.Loosely(t, in.GetProject(), should.Equal("chromium"))
				return &analysispb.QueryTestsResponse{TestIds: []string{resolvedID}}, nil
			}
			ruleURL := "https://luci-analysis.appspot.com/p/chromium/rules/0123456789abcdef0123456789abcdef"
			code, out, errOut := execTestHistoryCmd(thMock, fixedNow, ruleURL, "-test-substring", "WebAppBrowserTest#MyCase")
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, errOut, should.BeEmpty)
			assert.Loosely(t, out, should.ContainSubstring("Module Name:       //chrome/test:browser_tests"))
			assert.Loosely(t, out, should.ContainSubstring(`Resolved:  "WebAppBrowserTest#MyCase" -> `+resolvedID))

			// enrichExtractedWithTestID preserves pre-existing ModuleName when resolving to a legacy test ID without ':mod!'
			ext := &ids.ExtractedIDs{ModuleName: "existing_module"}
			enrichExtractedWithTestID(ext, "ninja://existing_module/Suite.Case")
			assert.Loosely(t, ext.ModuleName, should.Equal("existing_module"))
		})
	})
}
