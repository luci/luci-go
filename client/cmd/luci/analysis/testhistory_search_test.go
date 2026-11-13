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
	"go.chromium.org/luci/resultdb/pbutil"
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
			assert.Loosely(t, out, should.NotContainSubstring("Other matching test IDs:"))
		})

		t.Run("Auto-resolves unambiguous bounded-suffix match across GTest Suite.Case, C++ namespaces, JUnit, and same-module ninja legacy counterpart", func(t *ftt.Test) {
			const structuredGTest = `://chrome/test\:browser_tests!gtest::WebAppBrowserTest#MyCase`
			const legacyGTest = "ninja://chrome/test:browser_tests/WebAppBrowserTest.MyCase"
			const superstringGTest = `://chrome/test\:browser_tests!gtest::WebAppBrowserTest#MyCaseHelper`

			t.Run("Prefers structured ID over same-module legacy ninja ID and ignores superstrings", func(t *ftt.Test) {
				var queriedTestID string
				otherInteractiveGTest := `://chrome/test\:interactive_ui_tests!gtest::WebAppBrowserTest#MyCase/All.0`
				otherExtraGTest := `://chrome/test\:unit_tests!gtest::WebAppBrowserTest#MyCaseExtra`
				otherZetaGTest := `://chrome/test\:zeta_tests!gtest::WebAppBrowserTest#MyCaseZeta`
				thMock := &mockTestHistoryClient{
					queryTests: func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
						return &analysispb.QueryTestsResponse{
							TestIds: []string{legacyGTest, superstringGTest, otherInteractiveGTest, otherExtraGTest, otherZetaGTest, structuredGTest},
						}, nil
					},
					queryStats: func(ctx context.Context, in *analysispb.QueryTestHistoryStatsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryStatsResponse, error) {
						queriedTestID = in.TestId
						return &analysispb.QueryTestHistoryStatsResponse{
							Groups: []*analysispb.QueryTestHistoryStatsResponse_Group{
								{
									VariantHash:   "hash-1",
									VerdictCounts: &analysispb.QueryTestHistoryStatsResponse_Group_VerdictCounts{Flaky: 1, Passed: 5},
								},
							},
						}, nil
					},
					query: func(ctx context.Context, in *analysispb.QueryTestHistoryRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryResponse, error) {
						return &analysispb.QueryTestHistoryResponse{
							Verdicts: []*analysispb.TestVerdict{
								{
									TestId:       in.TestId,
									VariantHash:  "hash-1",
									InvocationId: "build-12345",
									StatusV2:     analysispb.TestVerdict_FLAKY,
								},
							},
						}, nil
					},
				}

				code, out, errOut := execTestHistoryCmd(thMock, fixedNow, "-project", "chromium", "-test-substring", "WebAppBrowserTest.MyCase")
				assert.Loosely(t, code, should.Equal(0))
				assert.Loosely(t, errOut, should.BeEmpty)
				assert.Loosely(t, queriedTestID, should.Equal(structuredGTest))
				assert.Loosely(t, out, should.ContainSubstring(
					`Resolved:  "WebAppBrowserTest.MyCase" -> `+structuredGTest+` (from 6 matching test IDs; pass -testid for another test)`))
				assert.Loosely(t, out, should.ContainSubstring(
					"Other matching test IDs:\n"+
						"- "+superstringGTest+"\n"+
						"- "+otherInteractiveGTest+"\n"+
						"- "+otherExtraGTest+"\n"+
						"  ... +2 more matching test IDs\n"))
				assert.Loosely(t, out, should.ContainSubstring(
					fmt.Sprintf("Next step:\n"+
						"  luci verdict get -invocationid build-12345 -testid %q -varianthash hash-1\n"+
						"  luci analysis test-history -project chromium -testid '%s'\n", structuredGTest, superstringGTest)))

				// When stats exist without recent verdicts, still surface Other matching test IDs and Next step.
				thMock.query = nil
				code, out, errOut = execTestHistoryCmd(thMock, fixedNow, "-project", "chromium", "-test-substring", "WebAppBrowserTest.MyCase")
				assert.Loosely(t, code, should.Equal(0))
				assert.Loosely(t, errOut, should.BeEmpty)
				assert.Loosely(t, out, should.ContainSubstring("No matching test verdicts found."))
				assert.Loosely(t, out, should.ContainSubstring("Other matching test IDs:\n- "+superstringGTest))
				assert.Loosely(t, out, should.ContainSubstring("Next step:\n  luci analysis test-history -project chromium -testid '"+superstringGTest+"'\n"))
			})

			t.Run("Lists alternate matching test IDs when auto-resolved ID has 0 verdicts in lookback window", func(t *ftt.Test) {
				thMock := &mockTestHistoryClient{
					queryTests: func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
						return &analysispb.QueryTestsResponse{
							TestIds: []string{legacyGTest, structuredGTest},
						}, nil
					},
				}

				code, out, errOut := execTestHistoryCmd(thMock, fixedNow, "-project", "chromium", "-test-substring", "WebAppBrowserTest.MyCase")
				assert.Loosely(t, code, should.Equal(0))
				assert.Loosely(t, errOut, should.BeEmpty)
				assert.Loosely(t, out, should.ContainSubstring("No test history found in the last 7 days."))
				assert.Loosely(t, out, should.ContainSubstring("Other matching test IDs:\n- "+legacyGTest))
				assert.Loosely(t, out, should.ContainSubstring("luci analysis test-history -project chromium -testid "+legacyGTest))

				// When an active non-counterpart structured variant also matched, prefer it ahead of the collapsed legacy ninja alias
				activeParamGTest := `://chrome/test\:browser_tests!gtest::WebAppBrowserTest#MyCase/All.0`
				thMock.queryTests = func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
					return &analysispb.QueryTestsResponse{
						TestIds: []string{legacyGTest, structuredGTest, activeParamGTest},
					}, nil
				}
				code, out, errOut = execTestHistoryCmd(thMock, fixedNow, "-project", "chromium", "-test-substring", "WebAppBrowserTest.MyCase")
				assert.Loosely(t, code, should.Equal(0))
				assert.Loosely(t, errOut, should.BeEmpty)
				assert.Loosely(t, out, should.ContainSubstring("Other matching test IDs:\n- "+activeParamGTest+"\n- "+legacyGTest))
				assert.Loosely(t, out, should.ContainSubstring("luci analysis test-history -project chromium -testid '"+activeParamGTest+"'"))
			})

			t.Run("Matches C++ namespaced GTest, parameterized GTest, Mocha/Jest trailing-slash CoarseName, and JUnit bounded suffixes", func(t *ftt.Test) {
				cppStructured := pbutil.EncodeTestID(pbutil.BaseTestIdentifier{
					ModuleName:   "//components:components_unittests",
					ModuleScheme: "gtest",
					FineName:     "autofill::FormDataTest",
					CaseName:     "MyCase",
				})
				cppSuperstring := pbutil.EncodeTestID(pbutil.BaseTestIdentifier{
					ModuleName:   "//components:components_unittests",
					ModuleScheme: "gtest",
					FineName:     "autofill::FormDataTest",
					CaseName:     "MyCaseWithExtra",
				})
				resolved, _ := rankAndResolveSubstringCandidates([]string{cppSuperstring, cppStructured}, "FormDataTest.MyCase", false, false)
				assert.Loosely(t, resolved, should.Equal(cppStructured))

				junitStructured := pbutil.EncodeTestID(pbutil.BaseTestIdentifier{
					ModuleName:   "//chrome/android:chrome_public_test_apk",
					ModuleScheme: "junit",
					CoarseName:   "org.chromium.chrome.browser",
					FineName:     "FooTest",
					CaseName:     "testBar",
				})
				junitLegacy := "ninja://chrome/android:chrome_public_test_apk/org.chromium.chrome.browser.FooTest#testBar"
				resolvedJUnit, _ := rankAndResolveSubstringCandidates([]string{junitLegacy, junitStructured}, "org.chromium.chrome.browser.FooTest#testBar", false, false)
				assert.Loosely(t, resolvedJUnit, should.Equal(junitStructured))

				// Parameterized GTest raw launcher name 'All/WebAppBrowserTest.BrowserDisplayNotInstallable/0'
				// collapses both raw JTR ninja IDs and result_adapter gtest rewritten ninja IDs in the same module.
				paramStructured := `://chrome/test\:browser_tests!gtest::WebAppBrowserTest#BrowserDisplayNotInstallable/All.0`
				paramOther := `://chrome/test\:browser_tests!gtest::WebAppBrowserTest#BrowserDisplayNotInstallable/All.1`
				paramLegacyRaw := "ninja://chrome/test:browser_tests/All/WebAppBrowserTest.BrowserDisplayNotInstallable/0"
				paramLegacyRewritten := "ninja://chrome/test:browser_tests/WebAppBrowserTest.BrowserDisplayNotInstallable/All.0"
				resolvedParam, _ := rankAndResolveSubstringCandidates(
					[]string{paramOther, paramLegacyRaw, paramLegacyRewritten, paramStructured},
					"All/WebAppBrowserTest.BrowserDisplayNotInstallable/0",
					false,
					false,
				)
				assert.Loosely(t, resolvedParam, should.Equal(paramStructured))

				// Type-parameterized GTest ('All/FooTest/1.DoesBar' and 'FooTest/1.DoesBar')
				typeParamStructured := `://chrome/test\:unit_tests!gtest::FooTest#DoesBar/All.1`
				typeParamOther := `://chrome/test\:unit_tests!gtest::FooTest#DoesBar/All.2`
				resolvedTypeParam, _ := rankAndResolveSubstringCandidates(
					[]string{typeParamOther, typeParamStructured},
					"All/FooTest/1.DoesBar",
					false,
					false,
				)
				assert.Loosely(t, resolvedTypeParam, should.Equal(typeParamStructured))
				resolvedTypeShort, _ := rankAndResolveSubstringCandidates(
					[]string{typeParamOther, typeParamStructured},
					"1.DoesBar",
					false,
					false,
				)
				assert.Loosely(t, resolvedTypeShort, should.Equal(typeParamStructured))

				// Mocha / Jest CoarseName with mandatory trailing '/' joins cleanly without '//'
				mochaStructured := `://third_party/chromium-bidi\:webdriver_bidi_unittests!mocha:src/modules/autofill/:AutofillProcessor.test.ts#AutofillProcessor:rethrows`
				mochaSuperstring := `://third_party/chromium-bidi\:webdriver_bidi_unittests!mocha:src/modules/autofill/:AutofillProcessor.test.ts#AutofillProcessor:rethrows other errors`
				mochaLegacy := "ninja://third_party/chromium-bidi:webdriver_bidi_unittests/src/modules/autofill/AutofillProcessor.test.ts#AutofillProcessor:rethrows"
				resolvedMocha, _ := rankAndResolveSubstringCandidates(
					[]string{mochaSuperstring, mochaLegacy, mochaStructured},
					"autofill/AutofillProcessor.test.ts#AutofillProcessor:rethrows",
					false,
					false,
				)
				assert.Loosely(t, resolvedMocha, should.Equal(mochaStructured))

				// WebGPU CTS CoarseName:FineName:CaseName colon display form
				webgpuStructured := `://chrome/test\:telemetry_gpu_integration_test!webgpucts:webgpu\:api,adapter:requestAdapter#powerPreference="low"`
				webgpuSuperstring := `://chrome/test\:telemetry_gpu_integration_test!webgpucts:webgpu\:api,adapter:requestAdapter#powerPreference="low";forceFallback=true`
				resolvedWebGPU, _ := rankAndResolveSubstringCandidates(
					[]string{webgpuSuperstring, webgpuStructured},
					`webgpu:api,adapter:requestAdapter:powerPreference="low"`,
					false,
					false,
				)
				assert.Loosely(t, resolvedWebGPU, should.Equal(webgpuStructured))
			})
		})

		t.Run("Disambiguates ambiguous -test-substring matches without fetching history", func(t *ftt.Test) {
			const browserTestsID = `://chrome/test\:browser_tests!gtest::WebAppBrowserTest#MyCase`
			const interactiveTestsID = `://chrome/test\:interactive_ui_tests!gtest::WebAppBrowserTest#MyCase`
			const legacyBrowserTestsID = "ninja://chrome/test:browser_tests/WebAppBrowserTest.MyCase"
			const superstringID = `://chrome/test\:browser_tests!gtest::WebAppBrowserTest#MyCaseHelper`

			t.Run("Ranks bounded-suffix structured IDs first and preserves filter flags in Next step", func(t *ftt.Test) {
				statsCalled := false
				thMock := &mockTestHistoryClient{
					queryTests: func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
						return &analysispb.QueryTestsResponse{
							TestIds: []string{"", superstringID, legacyBrowserTestsID, interactiveTestsID, browserTestsID, browserTestsID},
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
					"-test-substring", "WebAppBrowserTest.MyCase",
				)
				assert.Loosely(t, code, should.Equal(0))
				assert.Loosely(t, errOut, should.BeEmpty)
				assert.Loosely(t, statsCalled, should.BeFalse)
				assert.Loosely(t, out, should.ContainSubstring("Multiple matching test IDs found (4); specify exact -testid to view history:"))

				// Verify ranking: structured bounded-suffix (alphabetical) -> legacy bounded-suffix -> non-bounded superstring.
				expectedList := "- " + browserTestsID + "\n" +
					"- " + interactiveTestsID + "\n" +
					"- " + legacyBrowserTestsID + "\n" +
					"- " + superstringID + "\n"
				assert.Loosely(t, out, should.ContainSubstring(expectedList))
				assert.Loosely(t, out, should.ContainSubstring(
					"luci analysis test-history -project chromium -subrealm ci -variant builder:linux-rel -days 21 -limit 5 -testid '"+browserTestsID+"'"))
			})

			t.Run("Does not collapse cross-module, different-suite, or different-case legacy ninja IDs or truncated pages", func(t *ftt.Test) {
				// Cross-module legacy ninja ID (both non-parameterized and result_adapter gtest rewritten parameterized)
				crossModuleLegacy := "ninja://content/test:content_browsertests/WebAppBrowserTest.MyCase"
				resolved, _ := rankAndResolveSubstringCandidates([]string{browserTestsID, crossModuleLegacy}, "WebAppBrowserTest.MyCase", false, false)
				assert.Loosely(t, resolved, should.BeEmpty)

				paramBrowser := `://chrome/test\:browser_tests!gtest::WebAppBrowserTest#MyCase/All.0`
				crossModuleParamLegacy := "ninja://content/test:content_browsertests/WebAppBrowserTest.MyCase/All.0"
				resolved, _ = rankAndResolveSubstringCandidates([]string{paramBrowser, crossModuleParamLegacy}, "All/WebAppBrowserTest.MyCase/0", false, false)
				assert.Loosely(t, resolved, should.BeEmpty)

				// Bare 'MyCase' or '.MyCase' never falsely marks a parameterized GTest ('#MyCase/All.0' or '#MyCase/0')
				// as a bounded-suffix match via synthetic type-parameterized 'Suite/0.MyCase' display strings.
				suitePrefixInstall := `://chrome/test\:browser_tests!gtest::MyCase#Install`
				suitePrefixUninstall := `://chrome/test\:browser_tests!gtest::MyCase#Uninstall`
				otherSuiteParam := `://chrome/test\:unit_tests!gtest::OtherSuite#MyCase/All.0`
				resolved, _ = rankAndResolveSubstringCandidates([]string{suitePrefixInstall, suitePrefixUninstall, otherSuiteParam}, "MyCase", false, false)
				assert.Loosely(t, resolved, should.BeEmpty)
				resolved, _ = rankAndResolveSubstringCandidates([]string{superstringID, otherSuiteParam}, ".MyCase", false, false)
				assert.Loosely(t, resolved, should.BeEmpty)
				resolved, _ = rankAndResolveSubstringCandidates([]string{browserTestsID, otherSuiteParam}, "MyCase", false, false)
				assert.Loosely(t, resolved, should.Equal(browserTestsID))
				resolved, _ = rankAndResolveSubstringCandidates([]string{browserTestsID, otherSuiteParam}, ".MyCase", false, false)
				assert.Loosely(t, resolved, should.Equal(browserTestsID))

				// Same module, different suite where FineName ("BrowserTest") is a non-bounded substring of "WebAppBrowserTest"
				shortSuiteStructured := `://chrome/test\:browser_tests!gtest::BrowserTest#MyCase`
				resolved, _ = rankAndResolveSubstringCandidates([]string{shortSuiteStructured, legacyBrowserTestsID}, "MyCase", false, false)
				assert.Loosely(t, resolved, should.BeEmpty)

				// Same module, global vs C++ namespaced suite
				globalStructured := `://components\:components_unittests!gtest::FormDataTest#MyCase`
				namespacedLegacy := "ninja://components:components_unittests/autofill::FormDataTest.MyCase"
				resolved, _ = rankAndResolveSubstringCandidates([]string{globalStructured, namespacedLegacy}, "FormDataTest.MyCase", false, false)
				assert.Loosely(t, resolved, should.BeEmpty)

				// Same module, flat scheme with different directories (dirA/foo.py vs dirB/foo.py) must not collapse,
				// whereas exact same-path counterpart (dirA/foo.py) collapses cleanly.
				flatStructuredA := `://mod\:tests!flat::#dirA/foo.py`
				flatLegacyB := "ninja://mod:tests/dirB/foo.py"
				flatLegacyA := "ninja://mod:tests/dirA/foo.py"
				resolved, _ = rankAndResolveSubstringCandidates([]string{flatStructuredA, flatLegacyB}, "foo.py", false, false)
				assert.Loosely(t, resolved, should.BeEmpty)
				resolved, _ = rankAndResolveSubstringCandidates([]string{flatLegacyA, flatStructuredA}, "foo.py", false, false)
				assert.Loosely(t, resolved, should.Equal(flatStructuredA))

				// Same module & suite, parameterized GTests with different base case names queried by parameter suffix
				paramStructuredA := `://chrome/test\:browser_tests!gtest::WebAppBrowserTest#CaseA/All.MyFeature`
				paramLegacyB := "ninja://chrome/test:browser_tests/All/WebAppBrowserTest.CaseB/MyFeature"
				resolved, _ = rankAndResolveSubstringCandidates([]string{paramStructuredA, paramLegacyB}, "MyFeature", false, false)
				assert.Loosely(t, resolved, should.BeEmpty)

				// Zero bounded-suffix matches across 2 parameterized instantiations
				param0 := browserTestsID + "/All.0"
				param1 := browserTestsID + "/All.1"
				resolved, _ = rankAndResolveSubstringCandidates([]string{param0, param1}, "WebAppBrowserTest#MyCase", false, false)
				assert.Loosely(t, resolved, should.BeEmpty)

				// Delimiter-prefixed queries (#MyCase, .MyCase, :Suite#MyCase, !gtest::...) match on boundary
				// without silently favoring empty-FineName flat IDs or empty-CoarseName gtest IDs.
				flatEmptyFine := `://chrome/test\:other_tests!flat::#MyCase`
				resolved, _ = rankAndResolveSubstringCandidates([]string{browserTestsID, flatEmptyFine}, "#MyCase", false, false)
				assert.Loosely(t, resolved, should.BeEmpty)
				resolved, _ = rankAndResolveSubstringCandidates([]string{browserTestsID, superstringID}, "#MyCase", false, false)
				assert.Loosely(t, resolved, should.Equal(browserTestsID))
				resolved, _ = rankAndResolveSubstringCandidates([]string{browserTestsID, superstringID}, ".MyCase", false, false)
				assert.Loosely(t, resolved, should.Equal(browserTestsID))
				resolved, _ = rankAndResolveSubstringCandidates([]string{browserTestsID, superstringID}, "!gtest::WebAppBrowserTest#MyCase", false, false)
				assert.Loosely(t, resolved, should.Equal(browserTestsID))

				junitSameSuite := `://chrome/android\:junit_tests!junit:org.chromium.web:WebAppBrowserTest#MyCase`
				resolved, _ = rankAndResolveSubstringCandidates([]string{browserTestsID, junitSameSuite}, ":WebAppBrowserTest#MyCase", false, false)
				assert.Loosely(t, resolved, should.BeEmpty)

				// JUnit nested class '$' delimiter disambiguates top-level vs enclosed inner class
				junitTop := `://chrome/android\:junit_tests!junit:org.chromium.web:InnerTest#testMethod`
				junitInner := `://chrome/android\:junit_tests!junit:org.chromium.web:OuterTest$InnerTest#testMethod`
				resolved, _ = rankAndResolveSubstringCandidates([]string{junitTop, junitInner}, "InnerTest#testMethod", false, false)
				assert.Loosely(t, resolved, should.BeEmpty)

				// CoarseName/FineName.Case and FineName == "-" slash legacy counterparts collapse to structured ID
				coarseSlashStructured := `://mod\:tests!deqp:dir/sub:Suite#MyCase`
				coarseSlashLegacy := "ninja://mod:tests/dir/sub/Suite.MyCase"
				resolved, _ = rankAndResolveSubstringCandidates([]string{coarseSlashLegacy, coarseSlashStructured}, "sub/Suite.MyCase", false, false)
				assert.Loosely(t, resolved, should.Equal(coarseSlashStructured))

				dashFineStructured := `://mod\:tests!deqp:dir/sub:-#my_case`
				dashFineLegacy := "ninja://mod:tests/dir/sub/my_case"
				resolved, _ = rankAndResolveSubstringCandidates([]string{dashFineLegacy, dashFineStructured}, "sub/my_case", false, false)
				assert.Loosely(t, resolved, should.Equal(dashFineStructured))

				// Multi-component CaseName (ChromeOS tast scheme: ':tast!tast::arc#Boot:vm' vs 'tast.arc.Boot.vm')
				// marks structured ID as bounded so it ranks ahead of legacy 'tast.*' and disambiguates rather than resolving to legacy.
				tastStructured := ":tast!tast::arc#Boot:vm"
				tastStructuredSlow := ":tast!tast::arc#Boot:vm_slow"
				tastLegacy := "tast.arc.Boot.vm"
				resolved, rankedTast := rankAndResolveSubstringCandidates([]string{tastLegacy, tastStructured}, "tast.arc.Boot.vm", false, false)
				assert.Loosely(t, resolved, should.BeEmpty)
				assert.Loosely(t, rankedTast, should.Match([]string{tastStructured, tastLegacy}))
				resolved, _ = rankAndResolveSubstringCandidates([]string{tastStructured, tastStructuredSlow}, "tast.arc.Boot.vm", false, false)
				assert.Loosely(t, resolved, should.Equal(tastStructured))
				resolved, _ = rankAndResolveSubstringCandidates([]string{tastStructured, tastStructuredSlow}, "arc.Boot.vm", false, false)
				assert.Loosely(t, resolved, should.Equal(tastStructured))

				// Escaped ':' and '\\' in structured CaseName components (splitStructuredCaseComponents)
				escapedCompStructured := pbutil.EncodeTestID(pbutil.BaseTestIdentifier{
					ModuleName:   "tast",
					ModuleScheme: "tast",
					FineName:     "arc",
					CaseName:     `Boot\:sub\\x:vm`,
				})
				resolved, _ = rankAndResolveSubstringCandidates([]string{escapedCompStructured, tastStructuredSlow}, `arc.Boot:sub\x.vm`, false, false)
				assert.Loosely(t, resolved, should.Equal(escapedCompStructured))

				// Non-gtest slash CaseNames: Go subtests do not falsely match parent query as '.TestBar',
				// and webtest directory prefixes do not falsely resolve via GTest parameterized rewriting.
				goParent := ":mod!go::pkg/foo#TestBar"
				goSub1 := ":mod!go::pkg/foo#TestBar/sub1"
				goSubDot := ":mod!go::pkg/foo#TestBar/sub.two"
				resolved, _ = rankAndResolveSubstringCandidates([]string{goSub1, goSubDot, goParent}, "TestBar", false, false)
				assert.Loosely(t, resolved, should.Equal(goParent))

				wptDirA := ":mod!webtest::external/wpt#dir/a.html"
				wptDirOther := ":mod!webtest::external/wpt#dir_other/b.html"
				resolved, _ = rankAndResolveSubstringCandidates([]string{wptDirA, wptDirOther}, "dir", false, false)
				assert.Loosely(t, resolved, should.BeEmpty)

				// Escaped ':', '#', and '!' inside structured components do not falsely match structural
				// delimiter-prefixed queries, whereas literal '\\' in FineName before '#' still matches.
				escapedColonMod := `://chrome/test\:foo!gtest::Suite#MyCase`
				plainMod := `:foo!gtest::Suite#MyCase`
				resolved, _ = rankAndResolveSubstringCandidates([]string{escapedColonMod, plainMod}, ":foo!gtest::Suite#MyCase", false, false)
				assert.Loosely(t, resolved, should.Equal(plainMod))
				assert.Loosely(t, hasRawBoundedSuffix(`prefix\#MyCase`, "#MyCase", false), should.BeFalse)
				assert.Loosely(t, hasRawBoundedSuffix(`prefix\!MyCase`, "!MyCase", false), should.BeFalse)
				assert.Loosely(t, hasRawBoundedSuffix(`prefix\\#MyCase`, "#MyCase", false), should.BeTrue)

				escapedHashCase := `://mod!flat::#prefix\#MyCase`
				plainHashCase := `://mod!flat::#MyCase`
				resolved, _ = rankAndResolveSubstringCandidates([]string{escapedHashCase, plainHashCase}, "#MyCase", false, false)
				assert.Loosely(t, resolved, should.Equal(plainHashCase))
				assert.Loosely(t, isBoundedSuffixMatch(`://mod!flat::#prefix\!MyCase`, "!MyCase", false), should.BeFalse)

				escapedColonCase := `://mod!flat::#Boot\:vm`
				resolved, _ = rankAndResolveSubstringCandidates([]string{escapedColonCase, tastStructured}, ":vm", false, false)
				assert.Loosely(t, resolved, should.Equal(tastStructured))

				escapedColonFine := `://mod!junit:pkg:Other\:Suite#MyCase`
				plainFine := `://mod!junit:pkg:Suite#MyCase`
				resolved, _ = rankAndResolveSubstringCandidates([]string{escapedColonFine, plainFine}, ":Suite#MyCase", false, false)
				assert.Loosely(t, resolved, should.Equal(plainFine))

				trailingSlashFine := `://mod!gtest::dir\\#a\:b`
				assert.Loosely(t, isBoundedSuffixMatch(trailingSlashFine, "#a:b", false), should.BeTrue)

				// Page-truncated response never auto-resolves even with 1 bounded-suffix match
				thMock := &mockTestHistoryClient{
					queryTests: func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
						return &analysispb.QueryTestsResponse{
							TestIds:       []string{browserTestsID, superstringID},
							NextPageToken: "next-page",
						}, nil
					},
				}
				code, out, _ := execTestHistoryCmd(thMock, fixedNow, "-project", "chromium", "-test-substring", "WebAppBrowserTest#MyCase")
				assert.Loosely(t, code, should.Equal(0))
				assert.Loosely(t, out, should.ContainSubstring("Multiple matching test IDs found (2+, page cap reached); specify exact -testid to view history:"))

				// Caps disambiguation output at maxMatchingTestsShown (20) and suggestions at maxDidYouMeanSuggestions (10)
				many := make([]string, 23)
				for i := range many {
					many[i] = fmt.Sprintf("://mod\\:tests!gtest::Suite#Case%02d", i)
				}
				var disambigBuf bytes.Buffer
				printSubstringDisambiguation(&disambigBuf, "chromium", "", "Case", many, false, nil, false, 7, false, 10)
				assert.Loosely(t, disambigBuf.String(), should.ContainSubstring("... +3 more matching test IDs"))
				assert.Loosely(t, disambigBuf.String(), should.ContainSubstring(
					"luci analysis test-history -project chromium -testid '://mod\\:tests!gtest::Suite#Case00'\n"))

				var suggestBuf bytes.Buffer
				printDidYouMeanSuggestions(&suggestBuf, "Other matching test IDs:", many[:12], maxDidYouMeanSuggestions, "luci analysis test-history -project chromium -testid x")
				assert.Loosely(t, suggestBuf.String(), should.ContainSubstring("... +2 more matching test IDs"))
			})
		})

		t.Run("Supports -case-insensitive, automatic case-insensitive retry, delimiter-rewrite fallback, RPC errors, and zero-match output", func(t *ftt.Test) {
			const structuredID = `://chrome/test\:browser_tests!gtest::WebAppBrowserTest#MyCase`
			const superstringID = `://chrome/test\:browser_tests!gtest::WebAppBrowserTest#MyCaseHelper`

			var calls []bool
			thMock := &mockTestHistoryClient{
				queryTests: func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
					calls = append(calls, in.GetCaseInsensitive())
					if !in.GetCaseInsensitive() {
						return &analysispb.QueryTestsResponse{}, nil
					}
					return &analysispb.QueryTestsResponse{
						TestIds: []string{superstringID, structuredID},
					}, nil
				},
			}

			code, out, errOut := execTestHistoryCmd(thMock, fixedNow, "-project", "chromium", "-test-substring", "webappbrowsertest.mycase")
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, errOut, should.BeEmpty)
			assert.Loosely(t, calls, should.Match([]bool{false, true}))
			assert.Loosely(t, out, should.ContainSubstring(`Resolved:  "webappbrowsertest.mycase" -> `+structuredID))

			// Explicit -case-insensitive skips the exact-case call
			calls = nil
			code, _, errOut = execTestHistoryCmd(thMock, fixedNow, "-project", "chromium", "-test-substring", "webappbrowsertest.mycase", "-case-insensitive")
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, errOut, should.BeEmpty)
			assert.Loosely(t, calls, should.Match([]bool{true}))

			// Structured-delimiter fallback rewrites 'pkg.Class#method', 'Class.method', and 'ns::Suite#Case'
			// when test_metadata.name is unset or uses '#' instead of '.'.
			const androidJUnitID = `:CtsSdkExtensionsTestCases!junit:com.android.os.ext:SdkExtensionsTest#testBadArgument`
			const cppNamespacedID = `://components\:components_unittests!gtest::autofill\:\:FormDataTest#MyCase`
			delimMock := &mockTestHistoryClient{
				queryTests: func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
					switch in.GetTestIdSubstring() {
					case "com.android.os.ext:SdkExtensionsTest#testBadArgument":
						return &analysispb.QueryTestsResponse{TestIds: []string{androidJUnitID}}, nil
					case "SdkExtensionsTest#testBadArgument":
						return &analysispb.QueryTestsResponse{TestIds: []string{androidJUnitID}}, nil
					case `autofill\:\:FormDataTest#MyCase`:
						return &analysispb.QueryTestsResponse{TestIds: []string{cppNamespacedID}}, nil
					default:
						return &analysispb.QueryTestsResponse{}, nil
					}
				},
			}
			code, out, errOut = execTestHistoryCmd(delimMock, fixedNow, "-project", "android", "-test-substring", "com.android.os.ext.SdkExtensionsTest#testBadArgument")
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, errOut, should.BeEmpty)
			assert.Loosely(t, out, should.ContainSubstring(`Resolved:  "com.android.os.ext.SdkExtensionsTest#testBadArgument" -> `+androidJUnitID))

			code, out, errOut = execTestHistoryCmd(delimMock, fixedNow, "-project", "android", "-test-substring", "SdkExtensionsTest.testBadArgument")
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, errOut, should.BeEmpty)
			assert.Loosely(t, out, should.ContainSubstring(`Resolved:  "SdkExtensionsTest.testBadArgument" -> `+androidJUnitID))

			code, out, errOut = execTestHistoryCmd(delimMock, fixedNow, "-project", "chromium", "-test-substring", "autofill::FormDataTest#MyCase")
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, errOut, should.BeEmpty)
			assert.Loosely(t, out, should.ContainSubstring(`Resolved:  "autofill::FormDataTest#MyCase" -> `+cppNamespacedID))
			assert.Loosely(t, alternateStructuredSubstring("descriptor-not-found.https.html"), should.BeEmpty)
			assert.Loosely(t, alternateStructuredSubstring("tast.arc.Boot.vm"), should.BeEmpty)
			assert.Loosely(t, alternateStructuredSubstring("#foo.bar"), should.BeEmpty)

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
