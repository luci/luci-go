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

package recorder

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type fakeWriter struct {
	grpc.ClientStream // implements the rest of ByteStream_WriteClient.

	requestsToAccept int
	requests         []*bytestream.WriteRequest

	res    *bytestream.WriteResponse
	resErr error
}

func (*fakeWriter) CloseSend() error {
	return nil
}

func (w *fakeWriter) CloseAndRecv() (*bytestream.WriteResponse, error) {
	if w.res != nil || w.resErr != nil {
		return w.res, w.resErr
	}

	res := &bytestream.WriteResponse{}
	for _, req := range w.requests {
		res.CommittedSize += int64(len(req.Data))
	}
	return res, nil
}

func (w *fakeWriter) Send(req *bytestream.WriteRequest) error {
	w.requests = append(w.requests, proto.Clone(req).(*bytestream.WriteRequest))
	if len(w.requests) >= w.requestsToAccept {
		return io.EOF
	}
	return nil
}

type createArtifactRequest struct {
	// The URL path to access.
	URLPath string
	// The request contents (the artifact content).
	Content string
	// The content hash header value.
	ContentHash string
	// The content size header value.
	ContentSize int64
	// The update token header value.
	UpdateToken string
	// The content type header value.
	ContentType string
	// The test identifier header value.
	TestID *pb.TestIdentifier
	// The result identifier header value.
	ResultID string
	// The artifact ID header value.
	ArtifactID string
	// The artifact type header value.
	ArtifactType string
	// Whether PUT should be used instead of POST.
	UsePUT bool
}

func prepareCreateArtifactRequest(req createArtifactRequest) *http.Request {
	urlValue := "/" + req.URLPath
	// Create a URL whose EscapePath() returns URLPath.
	u := &url.URL{RawPath: urlValue}
	var err error
	u.Path, err = url.PathUnescape(urlValue)
	if err != nil {
		panic(err)
	}

	result := &http.Request{
		Header: http.Header{},
		URL:    u,
		Body:   io.NopCloser(strings.NewReader(req.Content)),
	}
	if req.ContentHash != "" {
		result.Header.Set(artifactContentHashHeaderKey, req.ContentHash)
	}
	if req.ContentSize != -1 {
		result.Header.Set(artifactContentSizeHeaderKey, strconv.FormatInt(req.ContentSize, 10))
	}
	if req.UpdateToken != "" {
		result.Header.Set(updateTokenHeaderKey, req.UpdateToken)
	}
	if req.ContentType != "" {
		result.Header.Set(artifactContentTypeHeaderKey, req.ContentType)
	}
	if req.TestID != nil {
		if req.TestID.ModuleName != "" {
			result.Header.Set(testModuleNameHeaderKey, url.PathEscape(req.TestID.ModuleName))
		}
		if req.TestID.ModuleScheme != "" {
			result.Header.Set(testModuleSchemeHeaderKey, url.PathEscape(req.TestID.ModuleScheme))
		}
		if req.TestID.ModuleVariant != nil {
			for k, v := range req.TestID.ModuleVariant.Def {
				result.Header.Add(testVariantHeaderKey, url.PathEscape(k+":"+v))
			}
		}
		if req.TestID.CoarseName != "" {
			result.Header.Set(testCoarseNameHeaderKey, url.PathEscape(req.TestID.CoarseName))
		}
		if req.TestID.FineName != "" {
			result.Header.Set(testFineNameHeaderKey, url.PathEscape(req.TestID.FineName))
		}
		if req.TestID.CaseName != "" {
			result.Header.Set(testCaseNameHeaderKey, url.PathEscape(req.TestID.CaseName))
		}
	}
	if req.ResultID != "" {
		result.Header.Set(resultIDHeaderKey, url.PathEscape(req.ResultID))
	}
	if req.ArtifactID != "" {
		result.Header.Set(artifactIDHeaderKey, url.PathEscape(req.ArtifactID))
	}
	if req.ArtifactType != "" {
		result.Header.Set(artifactTypeHeaderKey, req.ArtifactType)
	}
	return result
}

func TestCreateArtifact(t *testing.T) {
	// metric field values for Artifact table
	artMFVs := []any{string(spanutil.Artifacts), string(spanutil.Inserted), insert.TestRealm}

	ftt.Run(`CreateArtifact`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		store := tsmon.Store(ctx)
		err := config.SetServiceConfigForTesting(ctx, config.CreatePlaceholderServiceConfig())
		assert.NoErr(t, err)

		w := &fakeWriter{}
		writerCreated := false
		ach := &artifactCreationHandler{
			RBEInstance: "projects/example/instances/artifacts",
			NewCASWriter: func(context.Context) (bytestream.ByteStream_WriteClient, error) {
				assert.Loosely(t, writerCreated, should.BeFalse)
				writerCreated = true
				return w, nil
			},
			MaxArtifactContentStreamLength: 1024,
		}

		rootInvID := rootinvocations.ID("root-inv-id")
		wuIDJUnit := workunits.ID{
			RootInvocationID: "root-inv-id",
			WorkUnitID:       "work-unit:junit",
		}
		junitModuleID := &pb.ModuleIdentifier{
			ModuleName:   "//infra/junit_tests",
			ModuleScheme: "junit",
			ModuleVariant: pbutil.Variant(
				"a/b", "1",
				"c", "2",
			),
			ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("a/b", "1", "c", "2")),
		}
		legacyModuleID := &pb.ModuleIdentifier{
			ModuleName:    "legacy",
			ModuleScheme:  "legacy",
			ModuleVariant: &pb.Variant{},
		}

		// Create some sample work units and a sample root invocation.
		var ms []*spanner.Mutation
		ms = append(ms, insert.RootInvocationWithRootWorkUnit(rootinvocations.NewBuilder(rootInvID).Build())...)
		ms = append(ms, insert.WorkUnit(workunits.NewBuilder(rootInvID, "work-unit:junit").WithFinalizationState(pb.WorkUnit_ACTIVE).WithModuleID(junitModuleID).Build())...)
		ms = append(ms, insert.WorkUnit(workunits.NewBuilder(rootInvID, "work-unit:legacy").WithFinalizationState(pb.WorkUnit_ACTIVE).WithModuleID(legacyModuleID).Build())...)
		testutil.MustApply(ctx, t, ms...)

		tvID := &pb.TestIdentifier{
			ModuleName:    junitModuleID.ModuleName,
			ModuleScheme:  junitModuleID.ModuleScheme,
			ModuleVariant: junitModuleID.ModuleVariant,
			CoarseName:    "org.chromium.go.luci",
			FineName:      "ValidationTests",
			CaseName:      "FooBar",
		}
		tvIDWithOutputOnlyFields := proto.Clone(tvID).(*pb.TestIdentifier)
		pbutil.PopulateStructuredTestIdentifierHashes(tvIDWithOutputOnlyFields)

		send := func(ctx context.Context, req createArtifactRequest) *httptest.ResponseRecorder {
			rec := httptest.NewRecorder()
			c := &router.Context{
				Request: prepareCreateArtifactRequest(req).WithContext(ctx),
				Writer:  rec,
			}
			if req.UsePUT {
				// Legacy requests.
				ach.HandlePUT(c)
			} else {
				ach.HandlePOST(c)
			}
			return rec
		}

		updateToken, err := generateWorkUnitUpdateToken(ctx, wuIDJUnit)
		assert.NoErr(t, err)

		req := createArtifactRequest{
			URLPath:      pbutil.WorkUnitName(string(wuIDJUnit.RootInvocationID), wuIDJUnit.WorkUnitID) + "/artifacts",
			Content:      "hello",
			ContentHash:  "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", // sha256 hash of "hello".
			ContentSize:  5,
			UpdateToken:  updateToken,
			ContentType:  "text/plain",
			TestID:       tvID,
			ResultID:     "result-id",
			ArtifactID:   "artifact-id",
			ArtifactType: "COVERAGE_REPORT",
			UsePUT:       false,
		}

		// Each t.Run(...) runs isolated from the other cases, so there is no need to reset state
		// to avoid interference between test cases.
		t.Run(`request validation`, func(t *ftt.Test) {
			t.Run(`URL path`, func(t *ftt.Test) {
				t.Run(`invalid suffix`, func(t *ftt.Test) {
					// The path should be a work unit name + "/artifacts", not an artifact name.
					req.URLPath = "rootInvocations/root-inv-id/workUnits/work-unit/artifacts/a"
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`URL: expected suffix '/artifacts' for artifacts upload endpoint`))
				})
				t.Run(`invalid work unit`, func(t *ftt.Test) {
					req.URLPath = "rootInvocations/root-inv-id/workUnits/-invalid-/artifacts"
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`URL: bad work unit name: does not match pattern`))
				})
				t.Run(`invalid invocation`, func(t *ftt.Test) {
					// The path should be an invocation name, not an artifact name.
					req.URLPath = "invocations/-invalid-/artifacts"
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`URL: bad invocation name: does not match pattern`))
				})
				t.Run(`legacy endpoint`, func(t *ftt.Test) {
					req.UsePUT = true
					// The path should be an artifact name, not an invocation name.
					req.URLPath = "invocations/a"
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix("URL: bad artifact name: does not match pattern"))
				})
			})
			t.Run(`test ID`, func(t *ftt.Test) {
				t.Run(`required if ResultID specified`, func(t *ftt.Test) {
					req.TestID = nil
					req.ResultID = "result-id"
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`artifact: result_id: specified, but no test ID is specified; did you forget to set Test-Module-Name and other Test- headers?`))
				})
				t.Run(`module name`, func(t *ftt.Test) {
					t.Run(`unspecified`, func(t *ftt.Test) {
						req.TestID.ModuleName = ""
						rsp := send(ctx, req)
						assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
						assert.Loosely(t, rsp.Body.String(), should.HavePrefix("Test-Module-Name header is missing (only part of a test ID is specified)"))
					})
				})
				t.Run(`module scheme`, func(t *ftt.Test) {
					t.Run(`unspecified`, func(t *ftt.Test) {
						req.TestID.ModuleScheme = ""
						rsp := send(ctx, req)
						assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
						assert.Loosely(t, rsp.Body.String(), should.HavePrefix("Test-Module-Scheme header is missing (only part of a test ID is specified)"))
					})
					t.Run(`unrecognised scheme`, func(t *ftt.Test) {
						req.TestID.ModuleScheme = "unknown"
						rsp := send(ctx, req)
						assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
						assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`artifact: test_id_structured: module_scheme: scheme "unknown" is not a known scheme by the ResultDB deployment; see go/resultdb-schemes for instructions how to define a new scheme`))
					})
				})
				t.Run(`module variant`, func(t *ftt.Test) {
					t.Run(`unspecified`, func(t *ftt.Test) {
						// This can be valid, although in this case it is not because it mismatches the scheme requirements.
						req.TestID.ModuleVariant = nil
						rsp := send(ctx, req)
						assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
						assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`artifact: test_id_structured: module_variant: does not match parent work unit module_id.module_variant; got {}, want {"a/b":"1","c":"2"}`))
					})
					t.Run(`invalid`, func(t *ftt.Test) {
						req.TestID.ModuleVariant.Def["bad_key_\x01"] = "value"
						rsp := send(ctx, req)
						assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
						assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`Test-Module-Variant header: pair "bad_key_\x01:value": does not match pattern`))
					})
				})
				t.Run(`case name`, func(t *ftt.Test) {
					t.Run(`unspecified`, func(t *ftt.Test) {
						req.TestID.CaseName = ""
						req.UpdateToken = updateToken
						rsp := send(ctx, req)
						assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
						assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`Test-Case-Name header is missing (only part of a test ID is specified)`))
					})
				})
				t.Run(`mismatches scheme requirements`, func(t *ftt.Test) {
					// Coarse name is required by the scheme.
					req.TestID.CoarseName = ""
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`artifact: test_id_structured: coarse_name: required, please set a Package (scheme "junit")`))
				})
				t.Run(`mismatches module specified at the work unit-level`, func(t *ftt.Test) {
					req.TestID.ModuleName = " other value "
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`artifact: test_id_structured: module_name: does not match parent work unit module_id.module_name; got " other value ", want "//infra/junit_tests"`))
				})
			})
			t.Run(`result ID`, func(t *ftt.Test) {
				t.Run(`unspecified if test ID is specified`, func(t *ftt.Test) {
					req.ResultID = ""
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`Result-ID header is missing`))
				})
				t.Run(`invalid`, func(t *ftt.Test) {
					req.ResultID = " result-id "
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`artifact: result_id: does not match pattern`))
				})
			})
			t.Run(`artifact ID`, func(t *ftt.Test) {
				t.Run(`unspecified`, func(t *ftt.Test) {
					req.ArtifactID = ""
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.ContainSubstring("Artifact-ID header is missing"))
				})
				t.Run(`invalid`, func(t *ftt.Test) {
					req.ArtifactID = " artifact-id "
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`artifact: artifact_id: does not match pattern`))
				})
			})
			t.Run(`artifact type`, func(t *ftt.Test) {
				t.Run(`invalid`, func(t *ftt.Test) {
					req.ArtifactType = "invalid-type"
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`artifact: artifact_type: does not match pattern`))
				})
				t.Run(`too long`, func(t *ftt.Test) {
					req.ArtifactType = strings.Repeat("a", 151)
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`artifact: artifact_type: exceeds 150 bytes`))
				})
			})
			t.Run(`content type`, func(t *ftt.Test) {
				t.Run(`invalid`, func(t *ftt.Test) {
					req.ContentType = "\x00"
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`artifact: content_type: mime: no media type`))
				})
			})
			t.Run(`content hash`, func(t *ftt.Test) {
				t.Run(`missing`, func(t *ftt.Test) {
					req.ContentHash = ""
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`Content-Hash header is missing`))
				})
				t.Run(`invalid`, func(t *ftt.Test) {
					req.ContentHash = "invalid"
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`Content-Hash header value does not match`))
				})
				t.Run(`mismatch`, func(t *ftt.Test) {
					req.ContentHash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`Content-Hash header value "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" does not match the hash of the request body, "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"`))
				})
			})
			t.Run(`content size`, func(t *ftt.Test) {
				ach.MaxArtifactContentStreamLength = 2048
				t.Run(`missing`, func(t *ftt.Test) {
					req.ContentSize = -1 // -1 is a special value that cause send() to omit the header.
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`Content-Length header is missing`))
				})
				t.Run(`invalid`, func(t *ftt.Test) {
					req.ContentSize = -100
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					// 1024 is the liit
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`Content-Length header must be a value between 0 and 2048`))
				})
				t.Run(`too large`, func(t *ftt.Test) {
					req.ContentSize = 1 << 30
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`Content-Length header must be a value between 0 and 2048`))
				})
			})
		})
		t.Run(`request authorization`, func(t *ftt.Test) {
			t.Run(`missing update token`, func(t *ftt.Test) {
				req.UpdateToken = ""
				rsp := send(ctx, req)
				assert.Loosely(t, rsp.Code, should.Equal(http.StatusUnauthorized))
				assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`Update-Token header is missing`))
			})
			t.Run(`invalid update token`, func(t *ftt.Test) {
				req.UpdateToken = "invalid"
				rsp := send(ctx, req)
				assert.Loosely(t, rsp.Code, should.Equal(http.StatusForbidden))
				assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`invalid Update-Token header value`))
			})
			t.Run(`with invocations`, func(t *ftt.Test) {
				req.URLPath = "invocations/inv/artifacts"

				t.Run(`missing update token`, func(t *ftt.Test) {
					req.UpdateToken = ""
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusUnauthorized))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`Update-Token header is missing`))
				})
				t.Run(`invalid update token`, func(t *ftt.Test) {
					req.UpdateToken = "invalid"
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusForbidden))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`invalid Update-Token header value`))
				})
			})
			t.Run(`with legacy request construction`, func(t *ftt.Test) {
				req.UsePUT = true
				req.URLPath = "invocations/inv/artifacts/a"

				t.Run(`missing update token`, func(t *ftt.Test) {
					req.UpdateToken = ""
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusUnauthorized))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`Update-Token header is missing`))
				})
				t.Run(`invalid update token`, func(t *ftt.Test) {
					req.UpdateToken = "invalid"
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusForbidden))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`invalid Update-Token header value`))
				})
			})
		})
		t.Run(`with work units`, func(t *ftt.Test) {
			expectedArtifact := &pb.Artifact{
				Name:             pbutil.TestResultArtifactName(string(wuIDJUnit.RootInvocationID), wuIDJUnit.WorkUnitID, pbutil.TestIDFromStructuredTestIdentifier(tvID), "result-id", "artifact-id"),
				TestIdStructured: tvIDWithOutputOnlyFields,
				TestId:           pbutil.TestIDFromStructuredTestIdentifier(tvID),
				ResultId:         "result-id",
				ArtifactId:       "artifact-id",
				ContentType:      "text/plain",
				ArtifactType:     "COVERAGE_REPORT",
				SizeBytes:        5,
				HasLines:         true,
			}

			t.Run(`base case (success)`, func(t *ftt.Test) {
				rsp := send(ctx, req)
				assert.Loosely(t, rsp.Code, should.Equal(http.StatusCreated))
				assert.Loosely(t, rsp.Body.String(), should.Equal(""))

				// Verify with the database.
				got, err := artifacts.Read(span.Single(ctx), expectedArtifact.Name)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got.Artifact, should.Match(expectedArtifact))
				assert.Loosely(t, got.RBECASHash, should.Match(req.ContentHash))

				// Verify writes to RBE-CAS.
				assert.Loosely(t, w.requests, should.HaveLength(1))
				assert.Loosely(t, string(w.requests[0].Data), should.Equal("hello"))

				// Verify metric values.
				assert.Loosely(t, store.Get(ctx, spanutil.RowCounter, artMFVs), should.Equal(1))

				t.Run(`idempotent`, func(t *ftt.Test) {
					// Clear out RBE-CAS fake's request memory.
					w.requests = nil

					// Verify retrying the same request produces the same result.
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusCreated))

					// Verify with the database.
					got, err := artifacts.Read(span.Single(ctx), expectedArtifact.Name)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got.Artifact, should.Match(expectedArtifact))
					assert.Loosely(t, got.RBECASHash, should.Match(req.ContentHash))

					// No further request made to RBE-CAS as the artifacts
					// already exist.
					assert.Loosely(t, w.requests, should.BeNil)

					// Metric values should remain unchanged.
					assert.Loosely(t, store.Get(ctx, spanutil.RowCounter, artMFVs), should.Equal(1))
				})
			})
			t.Run(`work unit-level artifact`, func(t *ftt.Test) {
				req.TestID = nil
				req.ResultID = ""
				rsp := send(ctx, req)
				assert.Loosely(t, rsp.Code, should.Equal(http.StatusCreated))

				expectedArtifact.Name = pbutil.WorkUnitArtifactName(string(wuIDJUnit.RootInvocationID), wuIDJUnit.WorkUnitID, "artifact-id")
				expectedArtifact.TestId = ""
				expectedArtifact.TestIdStructured = nil
				expectedArtifact.ResultId = ""

				// Verify with the database.
				got, err := artifacts.Read(span.Single(ctx), expectedArtifact.Name)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got.Artifact, should.Match(expectedArtifact))
				assert.Loosely(t, got.RBECASHash, should.Match(req.ContentHash))
			})
			t.Run(`parent`, func(t *ftt.Test) {
				t.Run(`finalized`, func(t *ftt.Test) {
					testutil.MustApply(ctx, t, spanutil.UpdateMap("WorkUnits", map[string]any{
						"RootInvocationShardId": wuIDJUnit.RootInvocationShardID(),
						"WorkUnitId":            wuIDJUnit.WorkUnitID,
						"FinalizationState":     pb.WorkUnit_FINALIZED,
					}))
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`"rootInvocations/root-inv-id/workUnits/work-unit:junit" is not active`))
				})
				t.Run(`not found`, func(t *ftt.Test) {
					testutil.MustApply(ctx, t, spanner.Delete("WorkUnits", wuIDJUnit.Key()))
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusNotFound))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix((`"rootInvocations/root-inv-id/workUnits/work-unit:junit" not found`)))
				})
				t.Run(`no module ID set`, func(t *ftt.Test) {
					testutil.MustApply(ctx, t, spanutil.UpdateMap("WorkUnits", map[string]any{
						"RootInvocationShardId": wuIDJUnit.RootInvocationShardID(),
						"WorkUnitId":            wuIDJUnit.WorkUnitID,
						"ModuleName":            spanner.NullString{},
						"ModuleScheme":          spanner.NullString{},
						"ModuleVariant":         []string{},
						"ModuleVariantHash":     spanner.NullString{},
					}))

					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`artifact: to upload test results or test result artifacts, you must set the module_id on the parent work unit first`))
				})
			})
			t.Run(`different artifact already exists`, func(t *ftt.Test) {
				t.Run(`same storage format`, func(t *ftt.Test) {
					testutil.MustApply(ctx, t,
						spanutil.InsertMap("Artifacts", map[string]any{
							"InvocationId": wuIDJUnit.LegacyInvocationID(),
							"ParentId":     artifacts.ParentID(expectedArtifact.TestId, expectedArtifact.ResultId),
							"ArtifactId":   "artifact-id",
							"RBECASHash":   "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b851",
							"Size":         1,
						}),
					)

					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusConflict))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`artifact "rootInvocations/root-inv-id/workUnits/work-unit:junit/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id/artifacts/artifact-id" already exists with a different hash`))
				})
				t.Run(`different storage format`, func(t *ftt.Test) {
					// E.g. as created via BatchCreateArtifacts.
					testutil.MustApply(ctx, t,
						spanutil.InsertMap("Artifacts", map[string]any{
							"InvocationId": wuIDJUnit.LegacyInvocationID(),
							"ParentId":     artifacts.ParentID(expectedArtifact.TestId, expectedArtifact.ResultId),
							"ArtifactId":   "artifact-id",
							"GcsUri":       "gs://bucket/file.txt",
							"Size":         123,
						}),
					)

					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusConflict))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`artifact "rootInvocations/root-inv-id/workUnits/work-unit:junit/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id/artifacts/artifact-id" already exists with a different storage scheme (GCS vs non-GCS)`))
				})
			})
			t.Run(`RBE-CAS write`, func(t *ftt.Test) {
				t.Run(`chunked`, func(t *ftt.Test) {
					w.requestsToAccept = 2
					ach.bufSize = 3
					rec := send(ctx, req)
					assert.Loosely(t, rec.Code, should.Equal(http.StatusCreated))
					assert.Loosely(t, w.requests, should.HaveLength(2))
					assert.Loosely(t, string(w.requests[0].Data), should.Equal("hel"))
					assert.Loosely(t, string(w.requests[1].Data), should.Equal("lo"))
					assert.Loosely(t, store.Get(ctx, spanutil.RowCounter, artMFVs), should.Equal(1))
				})
				t.Run(`RBE-CAS stops request early`, func(t *ftt.Test) {
					w.requestsToAccept = 1
					w.res = &bytestream.WriteResponse{CommittedSize: 5} // blob already exists
					ach.bufSize = 1
					rec := send(ctx, req)
					assert.Loosely(t, rec.Code, should.Equal(http.StatusCreated))
					// Must not continue requests after receiving io.EOF in the first
					// request.
					assert.Loosely(t, w.requests, should.HaveLength(1))
					assert.Loosely(t, store.Get(ctx, spanutil.RowCounter, artMFVs), should.Equal(1))
				})
				t.Run(`RBE-CAS identifies digest or size mismatch`, func(t *ftt.Test) {
					w.resErr = status.Errorf(codes.InvalidArgument, "wrong hash")
					rec := send(ctx, req)
					assert.Loosely(t, rec.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rec.Body.String(), should.Equal("Content-Hash and/or Content-Length do not match the request body\n"))
					assert.Loosely(t, store.Get(ctx, spanutil.RowCounter, artMFVs), should.BeNil)
				})
			})
		})
		t.Run(`with invocations`, func(t *ftt.Test) {
			invID := invocations.ID("inv-id")
			// Create a sample invocation.
			testutil.MustApply(ctx, t, insert.Invocation(invID, pb.Invocation_ACTIVE, map[string]any{
				"ModuleName":        "//infra/junit_tests",
				"ModuleScheme":      "junit",
				"ModuleVariant":     pbutil.Variant("a/b", "1", "c", "2"),
				"ModuleVariantHash": pbutil.VariantHash(pbutil.Variant("a/b", "1", "c", "2")),
			}))

			updateToken, err = generateInvocationToken(ctx, invID)
			assert.NoErr(t, err)

			req.URLPath = "invocations/inv-id/artifacts"
			req.UpdateToken = updateToken

			expectedArtifact := &pb.Artifact{
				Name:             pbutil.LegacyTestResultArtifactName(string(invID), pbutil.TestIDFromStructuredTestIdentifier(tvID), "result-id", "artifact-id"),
				TestIdStructured: tvIDWithOutputOnlyFields,
				TestId:           pbutil.TestIDFromStructuredTestIdentifier(tvID),
				ResultId:         "result-id",
				ArtifactId:       "artifact-id",
				ContentType:      "text/plain",
				ArtifactType:     "COVERAGE_REPORT",
				SizeBytes:        5,
				HasLines:         true,
			}

			t.Run(`base case (success)`, func(t *ftt.Test) {
				rsp := send(ctx, req)
				assert.Loosely(t, rsp.Code, should.Equal(http.StatusCreated))
				assert.Loosely(t, rsp.Body.String(), should.Equal(""))

				// Verify with the database.
				got, err := artifacts.Read(span.Single(ctx), expectedArtifact.Name)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got.Artifact, should.Match(expectedArtifact))
				assert.Loosely(t, got.RBECASHash, should.Match(req.ContentHash))

				// Verify writes to RBE-CAS.
				assert.Loosely(t, w.requests, should.HaveLength(1))
				assert.Loosely(t, string(w.requests[0].Data), should.Equal("hello"))

				// Verify metric values.
				assert.Loosely(t, store.Get(ctx, spanutil.RowCounter, artMFVs), should.Equal(1))

				t.Run(`idempotent`, func(t *ftt.Test) {
					// Clear out RBE-CAS fake's request memory.
					w.requests = nil

					// Verify retrying the same request produces the same result.
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusCreated))

					// Verify with the database.
					got, err := artifacts.Read(span.Single(ctx), expectedArtifact.Name)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got.Artifact, should.Match(expectedArtifact))
					assert.Loosely(t, got.RBECASHash, should.Match(req.ContentHash))

					// No further request made to RBE-CAS as the artifacts
					// already exist.
					assert.Loosely(t, w.requests, should.BeNil)

					// Metric values should remain unchanged.
					assert.Loosely(t, store.Get(ctx, spanutil.RowCounter, artMFVs), should.Equal(1))
				})
			})
			t.Run(`legacy-form request`, func(t *ftt.Test) {
				req.UsePUT = true
				// Pass Test ID, Result ID and Artifact ID via URL, not via headers.
				req.URLPath = pbutil.LegacyTestResultArtifactName(string(invID), pbutil.TestIDFromStructuredTestIdentifier(tvID), "result-id", "artifact-id")
				req.TestID = nil
				req.ResultID = ""
				req.ArtifactID = ""

				rsp := send(ctx, req)
				// 204 (No Content) is the expected status code for successful PUT requests.
				assert.Loosely(t, rsp.Code, should.Equal(http.StatusNoContent))

				// Verify with the database.
				got, err := artifacts.Read(span.Single(ctx), expectedArtifact.Name)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got.Artifact, should.Match(expectedArtifact))
				assert.Loosely(t, got.RBECASHash, should.Match(req.ContentHash))
			})
			t.Run(`invocation-level artifact`, func(t *ftt.Test) {
				req.TestID = nil
				req.ResultID = ""

				expectedArtifact.Name = pbutil.LegacyInvocationArtifactName(string(invID), "artifact-id")
				expectedArtifact.TestId = ""
				expectedArtifact.TestIdStructured = nil
				expectedArtifact.ResultId = ""

				t.Run(`base case`, func(t *ftt.Test) {
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusCreated))

					// Verify with the database.
					got, err := artifacts.Read(span.Single(ctx), expectedArtifact.Name)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got.Artifact, should.Match(expectedArtifact))
					assert.Loosely(t, got.RBECASHash, should.Match(req.ContentHash))
				})
				t.Run(`legacy-form request`, func(t *ftt.Test) {
					req.UsePUT = true
					req.URLPath = pbutil.LegacyInvocationArtifactName(string(invID), "artifact-id")
					req.ArtifactID = "" // Do not pass artifact ID via header.

					rsp := send(ctx, req)
					// 204 (No Content) is the expected status code for successful PUT requests.
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusNoContent))

					// Verify with the database.
					got, err := artifacts.Read(span.Single(ctx), expectedArtifact.Name)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got.Artifact, should.Match(expectedArtifact))
					assert.Loosely(t, got.RBECASHash, should.Match(req.ContentHash))
				})
			})
			t.Run(`parent`, func(t *ftt.Test) {
				t.Run(`finalized`, func(t *ftt.Test) {
					testutil.MustApply(ctx, t, spanutil.UpdateMap("Invocations", map[string]any{
						"InvocationId": invID,
						"State":        pb.Invocation_FINALIZED,
					}))
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix(`"invocations/inv-id" is not active`))
				})
				t.Run(`not found`, func(t *ftt.Test) {
					testutil.MustApply(ctx, t, spanner.Delete("Invocations", invID.Key()))
					rsp := send(ctx, req)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusNotFound))
					assert.Loosely(t, rsp.Body.String(), should.HavePrefix((`invocations/inv-id not found`)))
				})
			})
		})
	})
}
