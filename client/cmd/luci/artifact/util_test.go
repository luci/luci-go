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

package artifact

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/validate"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestFormatArtifactNames(t *testing.T) {
	t.Parallel()

	ftt.Run(`FormatTestResultArtifactName`, t, func(t *ftt.Test) {
		name := FormatTestResultArtifactName("build-123", "test:foo#bar", "0", "stdout")
		assert.Loosely(t, name, should.Equal("invocations/build-123/tests/test:foo%23bar/results/0/artifacts/stdout"))
	})

	ftt.Run(`FormatTestResultWorkUnitArtifactName`, t, func(t *ftt.Test) {
		name := FormatTestResultWorkUnitArtifactName("ants-123", "wu-456", "test:foo#bar", "0", "stdout")
		assert.Loosely(t, name, should.Equal("rootInvocations/ants-123/workUnits/wu-456/tests/test:foo%23bar/results/0/artifacts/stdout"))
	})

	ftt.Run(`FormatWorkUnitArtifactName`, t, func(t *ftt.Test) {
		name := FormatWorkUnitArtifactName("ants-123", "wu-456", "stderr")
		assert.Loosely(t, name, should.Equal("rootInvocations/ants-123/workUnits/wu-456/artifacts/stderr"))
	})

	ftt.Run(`FormatInvocationArtifactName`, t, func(t *ftt.Test) {
		name := FormatInvocationArtifactName("build-123", "minidump", true)
		assert.Loosely(t, name, should.Equal("invocations/build-123/artifacts/minidump"))

		nameRoot := FormatInvocationArtifactName("build-123", "minidump", false)
		assert.Loosely(t, nameRoot, should.Equal("rootInvocations/build-123/artifacts/minidump"))

		nameSpecial := FormatInvocationArtifactName("build-123", "tradefed/pkg.Class#Method [0]?foo=bar.png", true)
		assert.Loosely(t, nameSpecial, should.Equal("invocations/build-123/artifacts/tradefed%2Fpkg.Class%23Method%20%5B0%5D%3Ffoo=bar.png"))
	})
}

func TestValidateArtifactFlags(t *testing.T) {
	t.Parallel()

	ftt.Run(`ValidateArtifactFlags`, t, func(t *ftt.Test) {
		t.Run(`missing artifact ID`, func(t *ftt.Test) {
			err := ValidateArtifactFlags(ParentTypeTestResult, "inv", "", "test", "0", "")
			assert.Loosely(t, err, should.ErrLike("flag -artifactid is required"))
		})

		t.Run(`test result parent missing required flags`, func(t *ftt.Test) {
			err := ValidateArtifactFlags(ParentTypeTestResult, "", "", "test", "0", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -testid, -resultid, and -artifactid are required"))

			err = ValidateArtifactFlags(ParentTypeTestResult, "inv", "", "", "0", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -testid, -resultid, and -artifactid are required"))

			err = ValidateArtifactFlags(ParentTypeTestResult, "inv", "", "test", "", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -testid, -resultid, and -artifactid are required"))
		})

		t.Run(`test result parent valid`, func(t *ftt.Test) {
			err := ValidateArtifactFlags(ParentTypeTestResult, "inv", "", "test", "0", "stdout")
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`work unit parent missing required flags`, func(t *ftt.Test) {
			err := ValidateArtifactFlags(ParentTypeWorkUnit, "", "wu-1", "", "", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -workunitid, and -artifactid are required"))

			err = ValidateArtifactFlags(ParentTypeWorkUnit, "inv", "", "", "", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -workunitid, and -artifactid are required"))
		})

		t.Run(`work unit parent valid`, func(t *ftt.Test) {
			err := ValidateArtifactFlags(ParentTypeWorkUnit, "inv", "wu-1", "", "", "stdout")
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`invocation parent missing required flags`, func(t *ftt.Test) {
			err := ValidateArtifactFlags(ParentTypeInvocation, "", "", "", "", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid and -artifactid are required"))
		})

		t.Run(`invocation parent valid`, func(t *ftt.Test) {
			err := ValidateArtifactFlags(ParentTypeInvocation, "inv", "", "", "", "stdout")
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`unknown parent missing required flags`, func(t *ftt.Test) {
			err := ValidateArtifactFlags(ParentTypeUnknown, "", "", "", "", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -artifactid, and either -workunitid (for work unit artifacts) or -testid and -resultid (for test result artifacts) are required"))

			err = ValidateArtifactFlags(ParentTypeUnknown, "", "", "test", "", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -testid, -resultid, and -artifactid are required for test result artifacts"))

			err = ValidateArtifactFlags(ParentTypeUnknown, "", "", "test", "0", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -testid, -resultid, and -artifactid are required for test result artifacts"))

			err = ValidateArtifactFlags(ParentTypeUnknown, "", "wu-1", "", "", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -workunitid, and -artifactid are required"))
		})

		t.Run(`unknown parent valid`, func(t *ftt.Test) {
			err := ValidateArtifactFlags(ParentTypeUnknown, "inv", "", "test", "0", "stdout")
			assert.Loosely(t, err, should.BeNil)

			err = ValidateArtifactFlags(ParentTypeUnknown, "inv", "wu-1", "", "", "stdout")
			assert.Loosely(t, err, should.BeNil)

			err = ValidateArtifactFlags(ParentTypeUnknown, "inv", "", "", "", "stdout")
			assert.Loosely(t, err, should.BeNil)
		})
	})
}

func TestResolveParentType(t *testing.T) {
	t.Parallel()

	ftt.Run(`ResolveParentType`, t, func(t *ftt.Test) {
		t.Run(`test result valid`, func(t *ftt.Test) {
			pt, err := ResolveParentType("inv", "", "test", "0")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pt, should.Equal(ParentTypeTestResult))

			pt, err = ResolveParentType("inv", "wu-1", "test", "0")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pt, should.Equal(ParentTypeTestResult))
		})

		t.Run(`test result missing flags`, func(t *ftt.Test) {
			_, err := ResolveParentType("inv", "", "test", "")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -testid, and -resultid are required for test result artifacts"))

			_, err = ResolveParentType("", "", "test", "0")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -testid, and -resultid are required for test result artifacts"))
		})

		t.Run(`work unit valid`, func(t *ftt.Test) {
			pt, err := ResolveParentType("inv", "wu-1", "", "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pt, should.Equal(ParentTypeWorkUnit))
		})

		t.Run(`work unit missing invocation ID`, func(t *ftt.Test) {
			_, err := ResolveParentType("", "wu-1", "", "")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid and -workunitid are required"))
		})

		t.Run(`invocation valid`, func(t *ftt.Test) {
			pt, err := ResolveParentType("inv", "", "", "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pt, should.Equal(ParentTypeInvocation))
		})

		t.Run(`neither specified`, func(t *ftt.Test) {
			_, err := ResolveParentType("", "", "", "")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid and either -workunitid (for work unit artifacts) or -testid and -resultid (for test result artifacts) are required"))
		})
	})
}

func TestResolveResourceAndArtifactNames(t *testing.T) {
	t.Parallel()

	ftt.Run(`ResolveTestResultResourceName`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`legacy mode queries test result`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryTestResults: func(ctx context.Context, in *rdbpb.QueryTestResultsRequest) (*rdbpb.QueryTestResultsResponse, error) {
					return &rdbpb.QueryTestResultsResponse{
						TestResults: []*rdbpb.TestResult{
							{
								Name:     "invocations/u-leaf/tests/test1/results/res1",
								TestId:   "test1",
								ResultId: "res1",
							},
						},
					}, nil
				},
			}
			res, err := ResolveTestResultResourceName(ctx, client, "build-123", "", "test1", "res1", true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("invocations/u-leaf/tests/test1/results/res1"))
		})

		t.Run(`explicit work unit`, func(t *ftt.Test) {
			res, err := ResolveTestResultResourceName(ctx, nil, "ants-i123", "wu-999", "test1", "res1", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("rootInvocations/ants-i123/workUnits/wu-999/tests/test1/results/res1"))
		})

		t.Run(`query verdict fallback`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *rdbpb.QueryTestVerdictsRequest) (*rdbpb.QueryTestVerdictsResponse, error) {
					return &rdbpb.QueryTestVerdictsResponse{
						TestVerdicts: []*rdbpb.TestVerdict{
							{
								TestId: "test1",
								Results: []*rdbpb.TestResult{
									{
										Name:     "rootInvocations/ants-i123/workUnits/discovered-wu/tests/test1/results/res1",
										TestId:   "test1",
										ResultId: "res1",
									},
								},
							},
						},
					}, nil
				},
			}
			res, err := ResolveTestResultResourceName(ctx, client, "ants-i123", "", "test1", "res1", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("rootInvocations/ants-i123/workUnits/discovered-wu/tests/test1/results/res1"))

			artName, err := ResolveArtifactResourceName(ctx, client, ParentTypeTestResult, "ants-i123", "", "test1", "res1", "snippet", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, artName, should.Equal("rootInvocations/ants-i123/workUnits/discovered-wu/tests/test1/results/res1/artifacts/snippet"))
		})

		t.Run(`not found`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *rdbpb.QueryTestVerdictsRequest) (*rdbpb.QueryTestVerdictsResponse, error) {
					return &rdbpb.QueryTestVerdictsResponse{}, nil
				},
			}
			_, err := ResolveTestResultResourceName(ctx, client, "ants-i123", "", "test1", "nonexistent", false)
			assert.Loosely(t, err, should.ErrLike("not found"))
		})

		t.Run(`ResolveTargetResourceName work unit`, func(t *ftt.Test) {
			res, err := ResolveTargetResourceName(ctx, nil, ParentTypeWorkUnit, "build-123", "wu-1", "", "", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("rootInvocations/build-123/workUnits/wu-1"))

			artName, err := ResolveArtifactResourceName(ctx, nil, ParentTypeWorkUnit, "build-123", "wu-1", "", "", "syslog", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, artName, should.Equal("rootInvocations/build-123/workUnits/wu-1/artifacts/syslog"))
		})

		t.Run(`ResolveTargetResourceName invocation`, func(t *ftt.Test) {
			res, err := ResolveTargetResourceName(ctx, nil, ParentTypeInvocation, "build-123", "", "", "", true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("invocations/build-123"))

			artName, err := ResolveArtifactResourceName(ctx, nil, ParentTypeInvocation, "build-123", "", "", "", "minidump", true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, artName, should.Equal("invocations/build-123/artifacts/minidump"))
		})

		t.Run(`ResolveTargetResourceName invocation unknown parent`, func(t *ftt.Test) {
			res, err := ResolveTargetResourceName(ctx, nil, ParentTypeUnknown, "build-123", "", "", "", true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("invocations/build-123"))

			artName, err := ResolveArtifactResourceName(ctx, nil, ParentTypeUnknown, "build-123", "", "", "", "minidump", true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, artName, should.Equal("invocations/build-123/artifacts/minidump"))
		})

		t.Run(`ResolveTargetResourceName with ParentTypeUnknown`, func(t *ftt.Test) {
			res, err := ResolveTargetResourceName(ctx, nil, ParentTypeUnknown, "build-123", "wu-1", "", "", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("rootInvocations/build-123/workUnits/wu-1"))

			res, err = ResolveTargetResourceName(ctx, nil, ParentTypeUnknown, "build-123", "wu-1", "test1", "res1", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("rootInvocations/build-123/workUnits/wu-1/tests/test1/results/res1"))
		})

		t.Run(`ResolveInvocationArtifact via QueryArtifacts`, func(t *ftt.Test) {
			t.Run(`single exact match on root invocation`, func(t *ftt.Test) {
				client := &mockResultDBClient{
					queryArtifacts: func(ctx context.Context, in *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error) {
						assert.Loosely(t, in.Parent, should.Equal("rootInvocations/ants-i05300010637202289"))
						assert.Loosely(t, in.Predicate.GetArtifactIdRegexp(), should.Equal(`host_log\.txt`))
						return &rdbpb.QueryArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{
									Name:       "rootInvocations/ants-i05300010637202289/workUnits/ants-wu56200010637202289/artifacts/host_log.txt",
									ArtifactId: "host_log.txt",
									FetchUrl:   "https://example.com/host_log.txt",
								},
							},
						}, nil
					},
				}
				var stderr bytes.Buffer
				art, err := ResolveInvocationArtifact(ctx, client, "I05300010637202289", "host_log.txt", &stderr)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, art.Name, should.Equal("rootInvocations/ants-i05300010637202289/workUnits/ants-wu56200010637202289/artifacts/host_log.txt"))
				assert.Loosely(t, stderr.String(), should.Equal(""))

				// Also via ResolveArtifactResourceName
				artName, err := ResolveArtifactResourceName(ctx, client, ParentTypeInvocation, "I05300010637202289", "", "", "", "host_log.txt", false)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, artName, should.Equal("rootInvocations/ants-i05300010637202289/workUnits/ants-wu56200010637202289/artifacts/host_log.txt"))
			})

			t.Run(`multiple matches returns disambiguation error with owning flags`, func(t *ftt.Test) {
				client := &mockResultDBClient{
					queryArtifacts: func(ctx context.Context, in *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error) {
						return &rdbpb.QueryArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{
									Name:         "rootInvocations/ants-i05300010637202289/workUnits/ants-wu56200010637202289/artifacts/logcat.txt",
									ArtifactId:   "logcat.txt",
									SizeBytes:    4096,
									ArtifactType: "HOST_LOG",
								},
								{
									Name:         "rootInvocations/ants-i05300010637202289/workUnits/ants-wu56200010637202290/tests/com.android.FooTest%23testBar/results/0/artifacts/logcat.txt",
									ArtifactId:   "logcat.txt",
									SizeBytes:    2048,
									ArtifactType: "LOGCAT",
								},
							},
						}, nil
					},
				}
				_, err := ResolveInvocationArtifact(ctx, client, "ants-i05300010637202289", "logcat.txt", io.Discard)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.ContainSubstring(`multiple artifacts (2) match "logcat.txt" in invocation "ants-i05300010637202289"; please re-run with one of the following flag sets to disambiguate:`))
				assert.Loosely(t, err.Error(), should.ContainSubstring(`1. -workunitid=ants-wu56200010637202289 (4.0 KiB (4096 bytes), HOST_LOG)`))
				assert.Loosely(t, err.Error(), should.ContainSubstring(`2. -workunitid=ants-wu56200010637202290 -testid='com.android.FooTest#testBar' -resultid=0 (2.0 KiB (2048 bytes), LOGCAT)`))
			})

			t.Run(`zero matches returns not found error`, func(t *ftt.Test) {
				client := &mockResultDBClient{
					queryArtifacts: func(ctx context.Context, in *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error) {
						return &rdbpb.QueryArtifactsResponse{}, nil
					},
				}
				_, err := ResolveInvocationArtifact(ctx, client, "ants-i05300010637202289", "missing.txt", io.Discard)
				assert.Loosely(t, err, should.ErrLike(`artifact "missing.txt" not found in invocation "ants-i05300010637202289"`))
			})

			t.Run(`fetch_artifact path-prefix fallback resolves basename and prints note`, func(t *ftt.Test) {
				calls := 0
				client := &mockResultDBClient{
					queryArtifacts: func(ctx context.Context, in *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error) {
						calls++
						if calls == 1 {
							// Exact match query returns empty
							return &rdbpb.QueryArtifactsResponse{}, nil
						}
						assert.Loosely(t, in.Predicate.GetArtifactIdRegexp(), should.Equal(`(?:.*/)?host_log_456\.txt`))
						return &rdbpb.QueryArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{
									Name:       "rootInvocations/ants-i05300010637202289/workUnits/ants-wu56200010637202289/artifacts/host_log_456.txt",
									ArtifactId: "host_log_456.txt",
									FetchUrl:   "https://example.com/host_log_456.txt",
								},
							},
						}, nil
					},
				}
				var stderr bytes.Buffer
				art, err := ResolveInvocationArtifact(ctx, client, "ants-i05300010637202289", "/builds/I05300010637202289/12345/target/inv_1/host_log_456.txt", &stderr)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, art.ArtifactId, should.Equal("host_log_456.txt"))
				assert.Loosely(t, stderr.String(), should.Equal("Note: resolved path \"/builds/I05300010637202289/12345/target/inv_1/host_log_456.txt\" to artifact \"host_log_456.txt\" (-workunitid=ants-wu56200010637202289)\n"))
			})

			t.Run(`bare filename fallback resolves directory-prefixed artifact`, func(t *ftt.Test) {
				calls := 0
				client := &mockResultDBClient{
					queryArtifacts: func(ctx context.Context, in *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error) {
						calls++
						if calls == 1 {
							assert.Loosely(t, in.Predicate.GetArtifactIdRegexp(), should.Equal(`ants-publish\.json`))
							return &rdbpb.QueryArtifactsResponse{}, nil
						}
						assert.Loosely(t, in.Predicate.GetArtifactIdRegexp(), should.Equal(`.+/ants-publish\.json`))
						return &rdbpb.QueryArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{
									Name:       "rootInvocations/ants-i05300010637202289/workUnits/ants-wu78200270526224282/artifacts/injectables-191970884%2Fants-publish.json",
									ArtifactId: "injectables-191970884/ants-publish.json",
									FetchUrl:   "https://example.com/ants-publish.json",
								},
							},
						}, nil
					},
				}
				var stderr bytes.Buffer
				art, err := ResolveInvocationArtifact(ctx, client, "I05300010637202289", "ants-publish.json", &stderr)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, art.ArtifactId, should.Equal("injectables-191970884/ants-publish.json"))
				assert.Loosely(t, stderr.String(), should.Equal("Note: resolved path \"ants-publish.json\" to artifact \"injectables-191970884/ants-publish.json\" (-workunitid=ants-wu78200270526224282)\n"))
			})

			t.Run(`fetch_artifact path-prefix fallback prefers multi-segment suffix match`, func(t *ftt.Test) {
				calls := 0
				client := &mockResultDBClient{
					queryArtifacts: func(ctx context.Context, in *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error) {
						calls++
						if calls == 1 {
							return &rdbpb.QueryArtifactsResponse{}, nil
						}
						return &rdbpb.QueryArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{
									Name:       "rootInvocations/ants-i05300010637202289/workUnits/ants-wu1/artifacts/inv_1%2Fhost_log.txt",
									ArtifactId: "inv_1/host_log.txt",
								},
								{
									Name:       "rootInvocations/ants-i05300010637202289/workUnits/ants-wu2/artifacts/inv_2%2Fhost_log.txt",
									ArtifactId: "inv_2/host_log.txt",
								},
							},
						}, nil
					},
				}
				art, err := ResolveInvocationArtifact(ctx, client, "ants-i05300010637202289", "/builds/I05300010637202289/12345/target/inv_1/host_log.txt", io.Discard)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, art.ArtifactId, should.Equal("inv_1/host_log.txt"))
			})

			t.Run(`multi-page pagination, tailMatches fallback, and error propagation`, func(t *ftt.Test) {
				client := &mockResultDBClient{
					queryArtifacts: func(ctx context.Context, in *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error) {
						assert.Loosely(t, in.Parent, should.Equal("rootInvocations/ants-i123"))
						if in.Predicate.GetArtifactIdRegexp() == `other_dir/sub/log\.txt` {
							return &rdbpb.QueryArtifactsResponse{}, nil
						}
						if in.PageToken == "" {
							return &rdbpb.QueryArtifactsResponse{
								Artifacts: []*rdbpb.Artifact{
									{
										Name:        "rootInvocations/ants-i123/workUnits/wu-1/artifacts/dir_a%2Flog.txt",
										ArtifactId:  "dir_a/log.txt",
										ContentType: "text/plain",
									},
								},
								NextPageToken: "page-2",
							}, nil
						}
						return &rdbpb.QueryArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{
									Name:       "rootInvocations/ants-i123/workUnits/wu-2/artifacts/dir_b%2Flog.txt",
									ArtifactId: "dir_b/log.txt",
								},
							},
						}, nil
					},
				}
				_, err := ResolveInvocationArtifact(ctx, client, "ants-i123", "other_dir/sub/log.txt", io.Discard)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.ContainSubstring(`1. -workunitid=wu-1 -artifactid=dir_a/log.txt (text/plain)`))
				assert.Loosely(t, err.Error(), should.ContainSubstring(`2. -workunitid=wu-2 -artifactid=dir_b/log.txt`))

				// PermissionDenied on initial QueryArtifacts
				permClient := &mockResultDBClient{
					queryArtifacts: func(ctx context.Context, in *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error) {
						return nil, status.Error(codes.PermissionDenied, "denied")
					},
				}
				_, err = ResolveInvocationArtifact(ctx, permClient, "ants-i123", "log.txt", io.Discard)
				assert.Loosely(t, err, should.ErrLike("missing ResultDB realm permission"))
			})

			t.Run(`more than DefaultMaxArtifacts matches indicates 100+ in disambiguation error`, func(t *ftt.Test) {
				client := &mockResultDBClient{
					queryArtifacts: func(ctx context.Context, in *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error) {
						arts := make([]*rdbpb.Artifact, DefaultMaxArtifacts)
						for i := range arts {
							arts[i] = &rdbpb.Artifact{
								Name:       "rootInvocations/ants-i123/workUnits/wu-1/artifacts/logcat.txt",
								ArtifactId: "logcat.txt",
							}
						}
						return &rdbpb.QueryArtifactsResponse{
							Artifacts:     arts,
							NextPageToken: "more",
						}, nil
					},
				}
				_, err := ResolveInvocationArtifact(ctx, client, "ants-i123", "logcat.txt", io.Discard)
				assert.Loosely(t, err, should.ErrLike(`multiple artifacts (100+) match "logcat.txt" in invocation "ants-i123" (showing first 100)`))
			})
		})
	})
}

func TestFetchHTTPByteRangeEmptyFile(t *testing.T) {
	t.Parallel()

	ftt.Run(`FetchHTTPByteRange on empty file`, t, func(t *ftt.Test) {
		ctx := context.Background()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serveHTTPRange(w, r, []byte(""))
		}))
		defer server.Close()

		var buf bytes.Buffer
		byteRange := &ByteRange{Start: -1, End: 65536}
		code, size, err := FetchHTTPByteRange(ctx, server.Client(), server.URL, byteRange, &buf)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, code, should.Equal(http.StatusRequestedRangeNotSatisfiable))
		assert.Loosely(t, size, should.Equal(0))
		assert.Loosely(t, buf.Len(), should.Equal(0))
	})
}

// chunkedReader yields a predetermined sequence of byte chunks on successive Read calls
// to deterministically test ring-buffer state transitions in copySuffix.
type chunkedReader struct {
	chunks [][]byte
	idx    int
	err    error
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if r.idx >= len(r.chunks) {
		if r.err != nil {
			return 0, r.err
		}
		return 0, io.EOF
	}
	n := copy(p, r.chunks[r.idx])
	r.idx++
	return n, nil
}

// failAfterWriter succeeds for the first allowedCalls Write calls and returns writeErr thereafter.
type failAfterWriter struct {
	allowedCalls int
	calls        int
	writeErr     error
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.calls >= w.allowedCalls {
		return 0, w.writeErr
	}
	w.calls++
	return len(p), nil
}

func TestCopySuffix(t *testing.T) {
	t.Parallel()

	ftt.Run(`copySuffix`, t, func(t *ftt.Test) {
		t.Run(`suffixLen <= 0 discards stream`, func(t *ftt.Test) {
			for _, suffixLen := range []int64{0, -5} {
				var buf bytes.Buffer
				nRead, err := copySuffix(&buf, strings.NewReader("abcdefghij"), suffixLen)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, nRead, should.Equal(int64(10)))
				assert.Loosely(t, buf.Len(), should.Equal(0))
			}
		})

		t.Run(`empty stream`, func(t *ftt.Test) {
			var buf bytes.Buffer
			nRead, err := copySuffix(&buf, strings.NewReader(""), 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nRead, should.Equal(int64(0)))
			assert.Loosely(t, buf.Len(), should.Equal(0))
		})

		t.Run(`single chunk >= suffixLen`, func(t *ftt.Test) {
			// Exact match (len(chunk) == suffixLen)
			var buf bytes.Buffer
			nRead, err := copySuffix(&buf, &chunkedReader{chunks: [][]byte{[]byte("abcdef")}}, 6)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nRead, should.Equal(int64(6)))
			assert.Loosely(t, buf.String(), should.Equal("abcdef"))

			// Strictly larger (len(chunk) > suffixLen)
			buf.Reset()
			nRead, err = copySuffix(&buf, &chunkedReader{chunks: [][]byte{[]byte("abcdefghij")}}, 4)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nRead, should.Equal(int64(10)))
			assert.Loosely(t, buf.String(), should.Equal("ghij"))
		})

		t.Run(`small chunks followed by large chunk >= suffixLen and trailing small chunk`, func(t *ftt.Test) {
			// Exercises: !wrapped accumulation -> chunk >= suffixLen superseding ring -> wrapped overwrite.
			r := &chunkedReader{
				chunks: [][]byte{
					[]byte("ab"),       // !wrapped: ring = "ab"
					[]byte("01234567"), // >= suffixLen (5): ring = "34567", writePos = 0, wrapped = true
					[]byte("XY"),       // wrapped overwrite at writePos 0: ring = "XY567", writePos = 2
				},
			}
			var buf bytes.Buffer
			nRead, err := copySuffix(&buf, r, 5)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nRead, should.Equal(int64(12)))
			assert.Loosely(t, buf.String(), should.Equal("567XY"))
		})

		t.Run(`multiple small chunks never reaching suffixLen (!wrapped at EOF)`, func(t *ftt.Test) {
			r := &chunkedReader{
				chunks: [][]byte{
					[]byte("ab"),
					[]byte("cd"),
					[]byte("ef"),
				},
			}
			var buf bytes.Buffer
			nRead, err := copySuffix(&buf, r, 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nRead, should.Equal(int64(6)))
			assert.Loosely(t, buf.String(), should.Equal("abcdef"))
		})

		t.Run(`multiple small chunks hitting suffixLen exactly`, func(t *ftt.Test) {
			// Exercises int64(len(ring)) == suffixLen transition inside !wrapped.
			r := &chunkedReader{
				chunks: [][]byte{
					[]byte("ab"),
					[]byte("cde"), // len(ring) becomes 5 == suffixLen -> wrapped = true, writePos = 0
				},
			}
			var buf bytes.Buffer
			nRead, err := copySuffix(&buf, r, 5)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nRead, should.Equal(int64(5)))
			assert.Loosely(t, buf.String(), should.Equal("abcde"))

			// Now with an additional chunk after hitting exact suffixLen.
			r2 := &chunkedReader{
				chunks: [][]byte{
					[]byte("ab"),
					[]byte("cde"), // wrapped = true, writePos = 0
					[]byte("fg"),  // overwrites "ab" -> "fgcde", writePos = 2
				},
			}
			buf.Reset()
			nRead, err = copySuffix(&buf, r2, 5)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nRead, should.Equal(int64(7)))
			assert.Loosely(t, buf.String(), should.Equal("cdefg"))
		})

		t.Run(`small chunks crossing suffixLen threshold and wrapping around multiple times`, func(t *ftt.Test) {
			// Exercises:
			// 1. !wrapped growing slice ("abc", len 3 < 5)
			// 2. !wrapped crossing suffixLen ("defg", 3+4 > 5) -> allocates fullRing "cdefg", writePos = 0, wrapped = true
			// 3. wrapped where len(chunk) <= firstPart ("hi", len 2 <= 5) -> ring = "hidefg"[0:5]="hideg", writePos = 2
			// 4. wrapped where len(chunk) > firstPart ("JKLM", len 4 > 5-2=3) -> splits across end ("JKL" at [2:5]) and start ("M" at [0:1]), writePos = 1
			// 5. wrapped where writePos reaches end of ring exactly ("NO", len 2, writePos 1->3; then "PQ", len 2 == 5-3, writePos wraps to 0)
			r := &chunkedReader{
				chunks: [][]byte{
					[]byte("abc"),
					[]byte("defg"),
					[]byte("hi"),
					[]byte("JKLM"),
				},
			}
			var buf bytes.Buffer
			nRead, err := copySuffix(&buf, r, 5)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nRead, should.Equal(int64(13)))
			assert.Loosely(t, buf.String(), should.Equal("iJKLM"))

			// Continue wrapping back to writePos == 0.
			rExactWrap := &chunkedReader{
				chunks: [][]byte{
					[]byte("abc"),
					[]byte("defg"),
					[]byte("hi"),
					[]byte("JKLM"), // writePos = 1
					[]byte("NO"),   // writePos = 3
					[]byte("PQ"),   // writePos = (3+2)%5 = 0
				},
			}
			buf.Reset()
			nRead, err = copySuffix(&buf, rExactWrap, 5)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nRead, should.Equal(int64(17)))
			assert.Loosely(t, buf.String(), should.Equal("MNOPQ"))
		})

		t.Run(`exhaustive sweep over chunk sizes and suffix lengths`, func(t *ftt.Test) {
			payload := make([]byte, 120)
			for i := range payload {
				payload[i] = byte('!' + (i % 90))
			}

			for _, chunkSize := range []int{1, 2, 3, 7, 13, 32, 64, 150} {
				var chunks [][]byte
				for off := 0; off < len(payload); off += chunkSize {
					end := off + chunkSize
					if end > len(payload) {
						end = len(payload)
					}
					chunks = append(chunks, payload[off:end])
				}

				for _, suffixLen := range []int64{1, 2, 5, 7, 13, 31, 32, 33, 64, 119, 120, 121, 200} {
					var buf bytes.Buffer
					r := &chunkedReader{chunks: chunks}
					nRead, err := copySuffix(&buf, r, suffixLen)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, nRead, should.Equal(int64(len(payload))))

					wantStart := int64(len(payload)) - suffixLen
					if wantStart < 0 {
						wantStart = 0
					}
					assert.Loosely(t, buf.Bytes(), should.Resemble(payload[wantStart:]))
				}
			}
		})

		t.Run(`error propagation from src and dst`, func(t *ftt.Test) {
			readErr := errors.New("simulated read failure")
			r := &chunkedReader{
				chunks: [][]byte{[]byte("abc"), []byte("def")},
				err:    readErr,
			}
			var buf bytes.Buffer
			nRead, err := copySuffix(&buf, r, 4)
			assert.Loosely(t, err, should.Equal(readErr))
			assert.Loosely(t, nRead, should.Equal(int64(6)))

			writeErr := errors.New("simulated write failure")
			// Fail on first Write when !wrapped
			_, err = copySuffix(&failAfterWriter{allowedCalls: 0, writeErr: writeErr}, &chunkedReader{chunks: [][]byte{[]byte("abc")}}, 10)
			assert.Loosely(t, err, should.Equal(writeErr))

			// Fail on first Write (ring[writePos:]) when wrapped
			_, err = copySuffix(&failAfterWriter{allowedCalls: 0, writeErr: writeErr}, &chunkedReader{chunks: [][]byte{[]byte("abcde"), []byte("fg")}}, 5)
			assert.Loosely(t, err, should.Equal(writeErr))

			// Fail on second Write (ring[:writePos]) when wrapped with writePos > 0
			_, err = copySuffix(&failAfterWriter{allowedCalls: 1, writeErr: writeErr}, &chunkedReader{chunks: [][]byte{[]byte("abcde"), []byte("fg")}}, 5)
			assert.Loosely(t, err, should.Equal(writeErr))
		})
	})
}

// serveHTTPRange is a mock HTTP handler that emulates standard HTTP Range requests.
func serveHTTPRange(w http.ResponseWriter, r *http.Request, content []byte) {
	rangeHdr := r.Header.Get("Range")
	total := int64(len(content))
	if rangeHdr == "" {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
		return
	}

	br, err := ParseByteRange(strings.TrimPrefix(rangeHdr, "bytes="))
	if err != nil || br == nil {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
		return
	}

	var start, end int64
	if br.Start < 0 && br.End >= 0 {
		// Suffix range: bytes=-N
		if br.End >= total {
			start = 0
			end = total - 1
		} else {
			start = total - br.End
			end = total - 1
		}
	} else if br.Start >= 0 && br.End < 0 {
		// bytes=S-
		start = br.Start
		end = total - 1
	} else {
		// bytes=S-E
		start = br.Start
		end = br.End
		if end >= total {
			end = total - 1
		}
	}

	if start < 0 {
		start = 0
	}
	if start > end || start >= total {
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}

	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))
	w.WriteHeader(http.StatusPartialContent)
	w.Write(content[start : end+1])
}

func TestCompileArtifactFilter(t *testing.T) {
	t.Parallel()

	ftt.Run(`CompileArtifactMatcher, CompileArtifactFilter, CompileArtifactGlob, and FilterArtifacts`, t, func(t *ftt.Test) {
		t.Run(`empty filter and glob return empty regexp and nil matcher`, func(t *ftt.Test) {
			serverRe, re, err := CompileArtifactMatcher("", "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, serverRe, should.Equal(""))
			assert.Loosely(t, re, should.BeNil)

			serverRe, re, err = CompileArtifactFilter("")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, serverRe, should.Equal(""))
			assert.Loosely(t, re, should.BeNil)

			globServerRe, globRe := CompileArtifactGlob("")
			assert.Loosely(t, globServerRe, should.Equal(""))
			assert.Loosely(t, globRe, should.BeNil)

			arts := []*rdbpb.Artifact{{ArtifactId: "a"}, {ArtifactId: "b"}}
			assert.Loosely(t, FilterArtifacts(arts, re), should.Resemble(arts))
		})

		t.Run(`-filter and -glob are mutually exclusive`, func(t *ftt.Test) {
			_, _, err := CompileArtifactMatcher("logcat", "*.txt")
			assert.Loosely(t, err, should.ErrLike("flags -filter and -glob are mutually exclusive"))

			serverReFilter, reFilter, err := CompileArtifactMatcher("logcat", "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, serverReFilter, should.Equal(`(?i).*(?:logcat).*`))
			assert.Loosely(t, reFilter.MatchString("foo_logcat.txt"), should.BeTrue)

			serverReGlob, reGlob, err := CompileArtifactMatcher("", "*.txt")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, serverReGlob, should.Equal(`(?i).*(?:\.txt)`))
			assert.Loosely(t, reGlob.MatchString("foo_logcat.txt"), should.BeTrue)
		})

		t.Run(`plain substring in -filter is case-insensitive and valid RegexpFragment`, func(t *ftt.Test) {
			serverRe, re, err := CompileArtifactFilter("logcat")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, validate.RegexpFragment(serverRe), should.BeNil)
			assert.Loosely(t, serverRe, should.Equal(`(?i).*(?:logcat).*`))
			assert.Loosely(t, re.MatchString("logcat.txt"), should.BeTrue)
			assert.Loosely(t, re.MatchString("stub/inv_1/FOO-LOGCAT-ON-FAILURE_123.txt"), should.BeTrue)
			assert.Loosely(t, re.MatchString("bugreport.zip"), should.BeFalse)
		})

		t.Run(`regex alternation and quantifiers in -filter are evaluated as RE2`, func(t *ftt.Test) {
			serverRe, re, err := CompileArtifactFilter("bugreport|host_log")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, validate.RegexpFragment(serverRe), should.BeNil)
			assert.Loosely(t, serverRe, should.Equal(`(?i).*(?:bugreport|host_log).*`))
			assert.Loosely(t, re.MatchString("dir/bugreport_2026.zip"), should.BeTrue)
			assert.Loosely(t, re.MatchString("dir/HOST_LOG_1.txt"), should.BeTrue)
			assert.Loosely(t, re.MatchString("logcat.txt"), should.BeFalse)

			// Ensure '?' quantifier is treated as RE2 (0 or 1 'u'), not a glob wildcard.
			serverReQuant, reQuant, err := CompileArtifactFilter("^colou?r$")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, validate.RegexpFragment(serverReQuant), should.BeNil)
			assert.Loosely(t, reQuant.MatchString("color"), should.BeTrue)
			assert.Loosely(t, reQuant.MatchString("colour"), should.BeTrue)
			assert.Loosely(t, reQuant.MatchString("coloxr"), should.BeFalse)
		})

		t.Run(`regex anchors ^ and $ in -filter are preserved inside valid RegexpFragment`, func(t *ftt.Test) {
			serverRe, re, err := CompileArtifactFilter("^ants-publish")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, validate.RegexpFragment(serverRe), should.BeNil)
			assert.Loosely(t, serverRe, should.Equal(`(?i).*(?:^ants-publish).*`))
			assert.Loosely(t, re.MatchString("ants-publish-fa3e9cd6_log.txt"), should.BeTrue)
			assert.Loosely(t, re.MatchString("injectables-191970884/ants-publish.json"), should.BeFalse)

			serverReEnd, reEnd, err := CompileArtifactFilter(`\.json$`)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, validate.RegexpFragment(serverReEnd), should.BeNil)
			assert.Loosely(t, serverReEnd, should.Equal(`(?i).*(?:\.json$).*`))
			assert.Loosely(t, reEnd.MatchString("injectables-191970884/ants-publish.json"), should.BeTrue)
			assert.Loosely(t, reEnd.MatchString("ants-publish.json.bak"), should.BeFalse)

			// Alternation where one branch has '^' and another has '$' preserves per-branch anchors.
			serverReAlt, reAlt, err := CompileArtifactFilter(`^ants-publish|\.json$`)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, validate.RegexpFragment(serverReAlt), should.BeNil)
			assert.Loosely(t, reAlt.MatchString("ants-publish-fa3e9cd6_log.txt"), should.BeTrue)
			assert.Loosely(t, reAlt.MatchString("dir/ants-publish-fa3e9cd6_log.txt"), should.BeFalse)
			assert.Loosely(t, reAlt.MatchString("injectables-191970884/req.json"), should.BeTrue)
			assert.Loosely(t, reAlt.MatchString("injectables-191970884/req.json.bak"), should.BeFalse)

			// Escaped trailing \$ is a literal '$', not an end anchor.
			serverReEsc, reEsc, err := CompileArtifactFilter(`price\$`)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, validate.RegexpFragment(serverReEsc), should.BeNil)
			assert.Loosely(t, serverReEsc, should.Equal(`(?i).*(?:price\$).*`))
			assert.Loosely(t, reEsc.MatchString("dir/price$_1.txt"), should.BeTrue)
		})

		t.Run(`invalid regex in -filter returns an error`, func(t *ftt.Test) {
			_, _, err := CompileArtifactFilter("[unclosed")
			assert.Loosely(t, err, should.ErrLike(`invalid -filter regular expression "[unclosed"`))
		})

		t.Run(`CompileArtifactGlob translates shell globs to valid RegexpFragment`, func(t *ftt.Test) {
			// 1. "*-logcat-on-failure_*.txt"
			serverRe1, re1 := CompileArtifactGlob("*-logcat-on-failure_*.txt")
			assert.Loosely(t, validate.RegexpFragment(serverRe1), should.BeNil)
			assert.Loosely(t, serverRe1, should.Equal(`(?i).*(?:-logcat-on-failure_.*\.txt)`))
			assert.Loosely(t, re1.MatchString("stub/cros-cts-test/inv_123/Test#method-logcat-on-failure_7009134500574910421.txt"), should.BeTrue)
			assert.Loosely(t, re1.MatchString("stub/cros-cts-test/inv_123/Test#method-logcat-on-failure_7009134500574910421.txt.gz"), should.BeFalse)
			assert.Loosely(t, re1.MatchString("stub/cros-cts-test/inv_123/Test#method-logcat-on-failure_7009134500574910421.zip"), should.BeFalse)
			assert.Loosely(t, re1.MatchString("logcat.txt"), should.BeFalse)

			// 2. "*detailed-events*.txt"
			serverRe2, re2 := CompileArtifactGlob("*detailed-events*.txt")
			assert.Loosely(t, validate.RegexpFragment(serverRe2), should.BeNil)
			assert.Loosely(t, serverRe2, should.Equal(`(?i).*(?:detailed-events.*\.txt)`))
			assert.Loosely(t, re2.MatchString("stub/inv_123/host-detailed-events_999.txt"), should.BeTrue)
			assert.Loosely(t, re2.MatchString("detailed-events.txt"), should.BeTrue)
			assert.Loosely(t, re2.MatchString("detailed-events.txt.gz"), should.BeFalse)
			assert.Loosely(t, re2.MatchString("detailed-events.json"), should.BeFalse)

			// 3. "*.json" matches files ending in .json and excludes .json_123.txt.gz
			serverReJson, reJson := CompileArtifactGlob("*.json")
			assert.Loosely(t, validate.RegexpFragment(serverReJson), should.BeNil)
			assert.Loosely(t, serverReJson, should.Equal(`(?i).*(?:\.json)`))
			assert.Loosely(t, reJson.MatchString("injectables-191970884/ants-publish.json"), should.BeTrue)
			assert.Loosely(t, reJson.MatchString("subprocess-DisplayDeviceInfo.deviceinfo.json_181180975.txt.gz"), should.BeFalse)

			// 4. "*" or "**" matches any artifact
			serverReStar, reStar := CompileArtifactGlob("*")
			assert.Loosely(t, validate.RegexpFragment(serverReStar), should.BeNil)
			assert.Loosely(t, serverReStar, should.Equal(`(?i).*`))
			assert.Loosely(t, reStar.MatchString("anything.txt"), should.BeTrue)

			// 5. Globs without leading '*': "bugreport_*.zip", "ants-publish*", "test?.log", and "test[0]_*.txt"
			serverRe3, re3 := CompileArtifactGlob("bugreport_*.zip")
			assert.Loosely(t, validate.RegexpFragment(serverRe3), should.BeNil)
			assert.Loosely(t, re3.MatchString("bugreport_12345.zip"), should.BeTrue)
			assert.Loosely(t, re3.MatchString("dir/bugreport_12345.zip"), should.BeTrue)
			assert.Loosely(t, re3.MatchString("bugreport.zip"), should.BeFalse)

			serverRePrefix, rePrefix := CompileArtifactGlob("ants-publish*")
			assert.Loosely(t, validate.RegexpFragment(serverRePrefix), should.BeNil)
			assert.Loosely(t, rePrefix.MatchString("ants-publish-fa3e9cd6_log.txt"), should.BeTrue)
			assert.Loosely(t, rePrefix.MatchString("injectables-191970884/ants-publish.json"), should.BeTrue)

			serverRe4, re4 := CompileArtifactGlob("test?.log")
			assert.Loosely(t, validate.RegexpFragment(serverRe4), should.BeNil)
			assert.Loosely(t, re4.MatchString("test1.log"), should.BeTrue)
			assert.Loosely(t, re4.MatchString("test12.log"), should.BeFalse)

			serverReBrackets, reBrackets := CompileArtifactGlob("test[0]_*.txt")
			assert.Loosely(t, validate.RegexpFragment(serverReBrackets), should.BeNil)
			assert.Loosely(t, reBrackets.MatchString("dir/test[0]_log.txt"), should.BeTrue)
			assert.Loosely(t, reBrackets.MatchString("dir/test0_log.txt"), should.BeFalse)
		})

		t.Run(`FilterArtifacts filters slice preserving order`, func(t *ftt.Test) {
			_, re, err := CompileArtifactFilter("bugreport|logcat")
			assert.Loosely(t, err, should.BeNil)
			arts := []*rdbpb.Artifact{
				{ArtifactId: "stdout.txt"},
				{ArtifactId: "BUGREPORT.zip"},
				{ArtifactId: "tradefed-log.txt"},
				{ArtifactId: "device_logcat_1.txt"},
			}
			filtered := FilterArtifacts(arts, re)
			assert.Loosely(t, len(filtered), should.Equal(2))
			assert.Loosely(t, filtered[0].ArtifactId, should.Equal("BUGREPORT.zip"))
			assert.Loosely(t, filtered[1].ArtifactId, should.Equal("device_logcat_1.txt"))
		})
	})
}
