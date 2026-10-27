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

package base

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestAntsHelpers(t *testing.T) {
	ftt.Run("AnTS URL and ID parsing", t, func(t *ftt.Test) {
		t.Run("IsAntsTestResultID", func(t *ftt.Test) {
			assert.Loosely(t, IsAntsTestResultID("TR29830335282615697"), should.BeTrue)
			assert.Loosely(t, IsAntsTestResultID("TR1"), should.BeTrue)
			assert.Loosely(t, IsAntsTestResultID("tr123"), should.BeTrue)
			assert.Loosely(t, IsAntsTestResultID("TR"), should.BeFalse)
			assert.Loosely(t, IsAntsTestResultID("123"), should.BeFalse)
			assert.Loosely(t, IsAntsTestResultID("invocations/TR123"), should.BeFalse)
		})

		t.Run("IsAntsInvocationID", func(t *ftt.Test) {
			assert.Loosely(t, IsAntsInvocationID("I77100010600769898"), should.BeTrue)
			assert.Loosely(t, IsAntsInvocationID("I1"), should.BeTrue)
			assert.Loosely(t, IsAntsInvocationID("ants-I77100010600769898"), should.BeTrue)
			assert.Loosely(t, IsAntsInvocationID("ants-i77100010600769898"), should.BeTrue)
			assert.Loosely(t, IsAntsInvocationID("I"), should.BeFalse)
		})

		t.Run("IsAntsWorkUnitID", func(t *ftt.Test) {
			assert.Loosely(t, IsAntsWorkUnitID("WU17100269020689387"), should.BeTrue)
			assert.Loosely(t, IsAntsWorkUnitID("WU1"), should.BeTrue)
			assert.Loosely(t, IsAntsWorkUnitID("ants-WU123"), should.BeTrue)
			assert.Loosely(t, IsAntsWorkUnitID("ants-wu83500269020198004"), should.BeTrue)
			assert.Loosely(t, IsAntsWorkUnitID("WU50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64"), should.BeTrue)
			assert.Loosely(t, IsAntsWorkUnitID("ants-wu78200270526224282-attempt__0:u-f5bacc67-4d7a-428d-a417-be4fe61e05a8"), should.BeTrue)
			assert.Loosely(t, IsAntsWorkUnitID("WU78200270526224282-attempt__0"), should.BeTrue)
			assert.Loosely(t, IsAntsWorkUnitID("ANTS-WU123-ATTEMPT__0:U-ABC"), should.BeTrue)
			assert.Loosely(t, IsAntsWorkUnitID("WU"), should.BeFalse)
		})

		t.Run("ExtractBareAntsWorkUnitID", func(t *ftt.Test) {
			assert.Loosely(t, ExtractBareAntsWorkUnitID("WU17100269020689387"), should.Equal("WU17100269020689387"))
			assert.Loosely(t, ExtractBareAntsWorkUnitID("wu17100269020689387"), should.Equal("WU17100269020689387"))
			assert.Loosely(t, ExtractBareAntsWorkUnitID("ants-wu17100269020689387"), should.Equal("WU17100269020689387"))
			assert.Loosely(t, ExtractBareAntsWorkUnitID("ANTS-WU17100269020689387"), should.Equal("WU17100269020689387"))
			assert.Loosely(t, ExtractBareAntsWorkUnitID("workUnits/WU123"), should.Equal("WU123"))
			assert.Loosely(t, ExtractBareAntsWorkUnitID("WU50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64"), should.Equal("WU50500270009982574"))
			assert.Loosely(t, ExtractBareAntsWorkUnitID("ants-wu78200270526224282-attempt__0:u-f5bacc67"), should.Equal("WU78200270526224282"))
			assert.Loosely(t, ExtractBareAntsWorkUnitID("invalid"), should.Equal(""))
			assert.Loosely(t, ExtractBareAntsWorkUnitID(""), should.Equal(""))
		})

		t.Run("ExtractAntsURLWorkUnit", func(t *ftt.Test) {
			assert.Loosely(t, ExtractAntsURLWorkUnit("https://android-build.corp.google.com/test_investigate/invocation/I77100010600769898/workunit/WU17100269020689387/"), should.Equal("WU17100269020689387"))
			assert.Loosely(t, ExtractAntsURLWorkUnit("https://android-build.corp.google.com/test_investigate/work_unit/WU89700270010105897"), should.Equal("WU89700270010105897"))
			assert.Loosely(t, ExtractAntsURLWorkUnit("https://android-build.googleplex.com/builds/tests/view?invocationId=I123&workUnitId=WU50500270009982574"), should.Equal("WU50500270009982574"))
			assert.Loosely(t, ExtractAntsURLWorkUnit("https://android-build.googleplex.com/builds/tests/view?wu_id=ants-wu123"), should.Equal("ants-wu123"))
			assert.Loosely(t, ExtractAntsURLWorkUnit("https://android-build.googleplex.com/builds/tests/view?invocationId=I123"), should.Equal(""))
		})

		t.Run("IsAntsURL and ExtractAntsURLComponents", func(t *ftt.Test) {
			url1 := "https://android-build.corp.google.com/test_investigate/invocation/I77100010600769898/test/TR29830335282615697/"
			assert.Loosely(t, IsAntsURL(url1), should.BeTrue)
			inv1, tr1 := ExtractAntsURLComponents(url1)
			assert.Loosely(t, inv1, should.Equal("I77100010600769898"))
			assert.Loosely(t, tr1, should.Equal("TR29830335282615697"))

			url2 := "https://android-build.corp.google.com/test_investigate/test/TR13830335277435395"
			assert.Loosely(t, IsAntsURL(url2), should.BeTrue)
			inv2, tr2 := ExtractAntsURLComponents(url2)
			assert.Loosely(t, inv2, should.Equal(""))
			assert.Loosely(t, tr2, should.Equal("TR13830335277435395"))

			url3 := "https://android-build.corp.google.com/test_investigate/invocation/I77100010600769898"
			assert.Loosely(t, IsAntsURL(url3), should.BeTrue)
			inv3, tr3 := ExtractAntsURLComponents(url3)
			assert.Loosely(t, inv3, should.Equal("I77100010600769898"))
			assert.Loosely(t, tr3, should.Equal(""))

			url4 := "https://android-build.googleplex.com/builds/tests/view?invocationId=I30800010616707848"
			assert.Loosely(t, IsAntsURL(url4), should.BeTrue)
			inv4, tr4 := ExtractAntsURLComponents(url4)
			assert.Loosely(t, inv4, should.Equal("I30800010616707848"))
			assert.Loosely(t, tr4, should.Equal(""))

			url5 := "https://android-build.corp.google.com/builds/tests/view?invocation_id=I30800010616707848&testResultId=TR29830335282615697"
			assert.Loosely(t, IsAntsURL(url5), should.BeTrue)
			inv5, tr5 := ExtractAntsURLComponents(url5)
			assert.Loosely(t, inv5, should.Equal("I30800010616707848"))
			assert.Loosely(t, tr5, should.Equal("TR29830335282615697"))

			nonAnts := "https://ci.chromium.org/ui/test-investigate/invocations/build-123"
			assert.Loosely(t, IsAntsURL(nonAnts), should.BeFalse)
		})

		t.Run("NormalizeWorkUnit", func(t *ftt.Test) {
			assert.Loosely(t, NormalizeWorkUnit("WU17100269020689387"), should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, NormalizeWorkUnit("ants-WU17100269020689387"), should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, NormalizeWorkUnit("ants-wu17100269020689387"), should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, NormalizeWorkUnit("workUnits/WU123"), should.Equal("ants-wu123"))
			assert.Loosely(t, NormalizeWorkUnit("WU50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64"), should.Equal("ants-wu50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64"))
			assert.Loosely(t, NormalizeWorkUnit("WU78200270526224282-attempt__0:u-f5bacc67"), should.Equal("ants-wu78200270526224282-attempt__0:u-f5bacc67"))
			assert.Loosely(t, NormalizeWorkUnit("root"), should.Equal("root"))
		})

		t.Run("NormalizeInvocation with AnTS IDs", func(t *ftt.Test) {
			assert.Loosely(t, NormalizeInvocation("I77100010600769898"), should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, NormalizeInvocation("ants-I77100010600769898"), should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, NormalizeInvocation("ants-i77100010600769898"), should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, NormalizeInvocation("invocations/I77100010600769898"), should.Equal("ants-i77100010600769898"))
		})
	})
}

func TestParseAntsTestResultJSON(t *testing.T) {
	ftt.Run("ParseAntsTestResultJSON", t, func(t *ftt.Test) {
		t.Run("Method test result with module parameters", func(t *ftt.Test) {
			jsonData := []byte(`{
					"testResultId": "TR13830335277435395",
					"invocationId": "I77100010600769898",
					"workUnitId": "WU17100269020689387",
					"testStatus": "PASS",
					"runNumber": 0,
					"attemptNumber": 1,
					"testIdentifierId": "a1a9ab35503045aa069b820825a597e3",
					"testIdentifier": {
						"module": "CellBroadcastServiceTests",
						"moduleParameters": [
							{"name": "module-abi", "value": "arm64-v8a"}
						],
						"testClass": "com.android.cellbroadcastservice.tests.GsmCellBroadcastHandlerTest",
						"method": "testResetAreaInfoWithDefaultSubChanged"
					},
					"aggregationDetail": {
						"aggregationLevel": "METHOD"
					}
				}`)

			info, err := ParseAntsTestResultJSON(jsonData)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info.TestResultID, should.Equal("TR13830335277435395"))
			assert.Loosely(t, info.InvocationID, should.Equal("I77100010600769898"))
			assert.Loosely(t, info.WorkUnitID, should.Equal("WU17100269020689387"))
			assert.Loosely(t, info.Status, should.Equal("pass"))
			assert.Loosely(t, info.RunNumber, should.Equal(0))
			assert.Loosely(t, info.AttemptNumber, should.Equal(1))
			assert.Loosely(t, info.TestIdentifierID, should.Equal("a1a9ab35503045aa069b820825a597e3"))
			assert.Loosely(t, info.ModuleName, should.Equal("CellBroadcastServiceTests"))
			assert.Loosely(t, info.ClassName, should.Equal("com.android.cellbroadcastservice.tests.GsmCellBroadcastHandlerTest"))
			assert.Loosely(t, info.MethodName, should.Equal("testResetAreaInfoWithDefaultSubChanged"))
			assert.Loosely(t, info.TestCase, should.Equal("CellBroadcastServiceTests#com.android.cellbroadcastservice.tests.GsmCellBroadcastHandlerTest.testResetAreaInfoWithDefaultSubChanged"))
			assert.Loosely(t, info.IsModuleError, should.BeFalse)
			assert.Loosely(t, info.AggregationType, should.Equal("METHOD"))
			assert.Loosely(t, len(info.ModuleParameters), should.Equal(1))
			assert.Loosely(t, info.ModuleParameters[0].Name, should.Equal("module-abi"))
			assert.Loosely(t, info.ModuleParameters[0].Value, should.Equal("arm64-v8a"))
		})

		t.Run("Module error result with aggregationLevel MODULE", func(t *ftt.Test) {
			jsonData := []byte(`{
					"testResultId": "TR29830335282615697",
					"invocationId": "I77100010600769898",
					"testStatus": "TEST_ERROR",
					"testIdentifier": {
						"module": "CellBroadcastReceiverMTS"
					},
					"aggregationDetail": {
						"aggregationLevel": "MODULE"
					}
				}`)

			info, err := ParseAntsTestResultJSON(jsonData)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info.TestResultID, should.Equal("TR29830335282615697"))
			assert.Loosely(t, info.InvocationID, should.Equal("I77100010600769898"))
			assert.Loosely(t, info.WorkUnitID, should.Equal(""))
			assert.Loosely(t, info.Status, should.Equal("testError"))
			assert.Loosely(t, info.ModuleName, should.Equal("CellBroadcastReceiverMTS"))
			assert.Loosely(t, info.ClassName, should.Equal(""))
			assert.Loosely(t, info.MethodName, should.Equal(""))
			assert.Loosely(t, info.TestCase, should.Equal("CellBroadcastReceiverMTS#."))
			assert.Loosely(t, info.IsModuleError, should.BeTrue)
			assert.Loosely(t, info.AggregationType, should.Equal("MODULE"))
		})

		t.Run("Module error result with aggregationType MODULE", func(t *ftt.Test) {
			jsonData := []byte(`{
					"testResultId": "TR29830335282615697",
					"invocationId": "I77100010600769898",
					"testStatus": "FAIL",
					"testIdentifier": {
						"module": "MyModule"
					},
					"aggregationDetail": {
						"aggregationType": "MODULE"
					}
				}`)

			info, err := ParseAntsTestResultJSON(jsonData)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info.IsModuleError, should.BeTrue)
			assert.Loosely(t, info.TestCase, should.Equal("MyModule#."))
			assert.Loosely(t, info.Status, should.Equal("fail"))
			assert.Loosely(t, info.AggregationType, should.Equal("MODULE"))
		})

		t.Run("Placeholder test result without module or class", func(t *ftt.Test) {
			jsonData := []byte(`{
					"testResultId": "TR98730375213734414",
					"invocationId": "I56900010616552061",
					"testStatus": "TEST_ERROR"
				}`)

			info, err := ParseAntsTestResultJSON(jsonData)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info.TestResultID, should.Equal("TR98730375213734414"))
			assert.Loosely(t, info.InvocationID, should.Equal("I56900010616552061"))
			assert.Loosely(t, info.TestCase, should.Equal("#."))
			assert.Loosely(t, info.IsModuleError, should.BeTrue)
		})

		t.Run("Result without module name but with class and method", func(t *ftt.Test) {
			jsonData := []byte(`{
					"testResultId": "TR36130379899461232",
					"invocationId": "I39300010618643739",
					"workUnitId": "WU28600269761659804",
					"testStatus": "FAIL",
					"testIdentifier": {
						"testClass": "SdvFWBaselineDHIOneVMTest",
						"method": "test_stop_start_vm"
					}
				}`)

			info, err := ParseAntsTestResultJSON(jsonData)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info.TestCase, should.Equal("#SdvFWBaselineDHIOneVMTest.test_stop_start_vm"))
			assert.Loosely(t, info.ModuleName, should.Equal(""))
			assert.Loosely(t, info.ClassName, should.Equal("SdvFWBaselineDHIOneVMTest"))
			assert.Loosely(t, info.MethodName, should.Equal("test_stop_start_vm"))
			assert.Loosely(t, info.IsModuleError, should.BeFalse)
		})

		t.Run("Invalid or empty JSON", func(t *ftt.Test) {
			_, errEmpty := ParseAntsTestResultJSON([]byte(""))
			assert.Loosely(t, errEmpty, should.NotBeNil)

			_, errMalformed := ParseAntsTestResultJSON([]byte("{not-json"))
			assert.Loosely(t, errMalformed, should.NotBeNil)

			_, errNoFields := ParseAntsTestResultJSON([]byte("{}"))
			assert.Loosely(t, errNoFields, should.NotBeNil)
		})
	})
}

func TestResolveAntsTestResult(t *testing.T) {
	ftt.Run("ResolveAntsTestResult", t, func(t *ftt.Test) {
		t.Run("200 OK method test result with header validation", func(t *ftt.Test) {
			// Simulates AnTS v4 OnePlatform API returning a method-level test result
			// and validates that request path, Accept header, and X-Goog-User-Project header are sent.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Loosely(t, r.Method, should.Equal("GET"))
				assert.Loosely(t, r.URL.Path, should.Equal("/v4/tests/TR13830335277435395"))
				assert.Loosely(t, r.Header.Get("X-Goog-User-Project"), should.Equal(AntsUserProject))
				assert.Loosely(t, r.Header.Get("Accept"), should.Equal("application/json"))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{
						"testResultId": "TR13830335277435395",
						"invocationId": "I77100010600769898",
						"workUnitId": "WU17100269020689387",
						"testStatus": "PASS",
						"testIdentifier": {
							"module": "CellBroadcastServiceTests",
							"testClass": "com.android.cellbroadcastservice.tests.GsmCellBroadcastHandlerTest",
							"method": "testResetAreaInfoWithDefaultSubChanged"
						}
					}`))
			}))
			defer srv.Close()

			ctx := WithAntsHTTPClient(context.Background(), srv.Client(), srv.URL)
			info, err := ResolveAntsTestResult(ctx, "TR13830335277435395")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info.TestResultID, should.Equal("TR13830335277435395"))
			assert.Loosely(t, info.InvocationID, should.Equal("I77100010600769898"))
			assert.Loosely(t, info.WorkUnitID, should.Equal("WU17100269020689387"))
			assert.Loosely(t, info.Status, should.Equal("pass"))
			assert.Loosely(t, info.ModuleName, should.Equal("CellBroadcastServiceTests"))
		})

		t.Run("200 OK module error aggregate", func(t *ftt.Test) {
			// Simulates AnTS v4 OnePlatform API returning a module-level failure aggregate
			// with aggregationLevel MODULE and empty class/method.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{
						"testResultId": "TR29830335282615697",
						"invocationId": "I77100010600769898",
						"testStatus": "TEST_ERROR",
						"testIdentifier": {
							"module": "CellBroadcastReceiverMTS"
						},
						"aggregationDetail": {
							"aggregationLevel": "MODULE"
						}
					}`))
			}))
			defer srv.Close()

			ctx := WithAntsHTTPClient(context.Background(), srv.Client(), srv.URL)
			info, err := ResolveAntsTestResult(ctx, "TR29830335282615697")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info.TestResultID, should.Equal("TR29830335282615697"))
			assert.Loosely(t, info.IsModuleError, should.BeTrue)
			assert.Loosely(t, info.TestCase, should.Equal("CellBroadcastReceiverMTS#."))
		})

		t.Run("404 Not Found error handling", func(t *ftt.Test) {
			// Simulates AnTS OnePlatform 404 NOT_FOUND response envelope.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{
						"error": {
							"code": 404,
							"message": "Requested entity was not found.",
							"status": "NOT_FOUND"
						}
					}`))
			}))
			defer srv.Close()

			ctx := WithAntsHTTPClient(context.Background(), srv.Client(), srv.URL)
			_, err := ResolveAntsTestResult(ctx, "TR00000000000000000")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("not found"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("404"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("Requested entity was not found."))
		})

		t.Run("401 Unauthorized permission denied error handling", func(t *ftt.Test) {
			// Simulates OnePlatform 401 UNAUTHENTICATED error response envelope.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{
						"error": {
							"code": 401,
							"message": "Request is missing required authentication credential.",
							"status": "UNAUTHENTICATED"
						}
					}`))
			}))
			defer srv.Close()

			ctx := WithAntsHTTPClient(context.Background(), srv.Client(), srv.URL)
			_, err := ResolveAntsTestResult(ctx, "TR13830335277435395")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("permission denied"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("401"))
			assert.Loosely(t, err.Error(), should.ContainSubstring(AndroidAccessHint))
		})

		t.Run("403 Forbidden permission denied error handling", func(t *ftt.Test) {
			// Simulates OnePlatform 403 PERMISSION_DENIED error response envelope.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{
						"error": {
							"code": 403,
							"message": "The caller does not have permission",
							"status": "PERMISSION_DENIED"
						}
					}`))
			}))
			defer srv.Close()

			ctx := WithAntsHTTPClient(context.Background(), srv.Client(), srv.URL)
			_, err := ResolveAntsTestResult(ctx, "TR13830335277435395")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("permission denied"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("403"))
			assert.Loosely(t, err.Error(), should.ContainSubstring(AndroidAccessHint))
		})

		t.Run("Invalid AnTS test result ID format", func(t *ftt.Test) {
			_, err := ResolveAntsTestResult(context.Background(), "invalid-id")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("invalid AnTS test result ID"))
		})
	})
}

func TestResolveAntsWorkUnit(t *testing.T) {
	ftt.Run("ResolveAntsWorkUnit", t, func(t *ftt.Test) {
		t.Run("200 OK task work unit with ResultDB properties", func(t *ftt.Test) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Loosely(t, r.Method, should.Equal("GET"))
				assert.Loosely(t, r.URL.Path, should.Equal("/v4/workunits/WU50500270009982574"))
				assert.Loosely(t, r.Header.Get("X-Goog-User-Project"), should.Equal(AntsUserProject))
				assert.Loosely(t, r.Header.Get("Accept"), should.Equal("application/json"))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{
						"id": "WU50500270009982574",
						"name": "test-task-1",
						"type": "TFC_COMMAND_TASK",
						"invocationId": "I71400010624864031",
						"state": "COMPLETED",
						"properties": [
							{"name": "resultdb_root_invocation_id", "value": "ants-i71400010624864031"},
							{"name": "resultdb_work_unit_id", "value": "ants-wu50500270009982574"}
						]
					}`))
			}))
			defer srv.Close()

			ctx := WithAntsHTTPClient(context.Background(), srv.Client(), srv.URL)
			info, err := ResolveAntsWorkUnit(ctx, "WU50500270009982574")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info.WorkUnitID, should.Equal("WU50500270009982574"))
			assert.Loosely(t, info.InvocationID, should.Equal("I71400010624864031"))
			assert.Loosely(t, info.Type, should.Equal("TFC_COMMAND_TASK"))
			assert.Loosely(t, info.ResultDBRootInvocationID, should.Equal("ants-i71400010624864031"))
			assert.Loosely(t, info.ResultDBWorkUnitID, should.Equal("ants-wu50500270009982574"))
		})

		t.Run("200 OK test run with ancestor walk (child -> module -> task)", func(t *ftt.Test) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v4/workunits/WU89700270010105897":
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{
							"id": "WU89700270010105897",
							"name": "x86_64 bluetooth_test_gd_unit",
							"type": "TF_TEST_RUN",
							"parentId": "WU97500270010256987",
							"invocationId": "I71400010624864031",
							"state": "ERROR"
						}`))
				case "/v4/workunits/WU97500270010256987":
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{
							"id": "WU97500270010256987",
							"name": "bluetooth_test_gd_unit",
							"type": "TF_MODULE",
							"parentId": "WU50500270009982574",
							"invocationId": "I71400010624864031",
							"state": "COMPLETED",
							"moduleInfo": {
								"name": "bluetooth_test_gd_unit",
								"moduleParameters": [
									{"name": "module-abi", "value": "x86_64"}
								]
							}
						}`))
				case "/v4/workunits/WU50500270009982574":
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{
							"id": "WU50500270009982574",
							"name": "test-task-1",
							"type": "TFC_COMMAND_TASK",
							"invocationId": "I71400010624864031",
							"state": "COMPLETED",
							"properties": [
								{"name": "resultdb_root_invocation_id", "value": "ants-i71400010624864031"},
								{"name": "resultdb_work_unit_id", "value": "ants-wu50500270009982574"}
							]
						}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			ctx := WithAntsHTTPClient(context.Background(), srv.Client(), srv.URL)
			info, err := ResolveAntsWorkUnit(ctx, "WU89700270010105897")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info.WorkUnitID, should.Equal("WU89700270010105897"))
			assert.Loosely(t, info.InvocationID, should.Equal("I71400010624864031"))
			assert.Loosely(t, info.Type, should.Equal("TF_TEST_RUN"))
			assert.Loosely(t, info.ModuleName, should.Equal("bluetooth_test_gd_unit"))
			assert.Loosely(t, len(info.ModuleParameters), should.Equal(1))
			assert.Loosely(t, info.ModuleParameters[0].Value, should.Equal("x86_64"))
			assert.Loosely(t, info.ResultDBRootInvocationID, should.Equal("ants-i71400010624864031"))
			assert.Loosely(t, info.ResultDBWorkUnitID, should.Equal("ants-wu50500270009982574"))
		})

		t.Run("404 Not Found error handling", func(t *ftt.Test) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{
						"error": {
							"code": 404,
							"message": "WorkUnit WU99999999999999999 not found.",
							"status": "NOT_FOUND"
						}
					}`))
			}))
			defer srv.Close()

			ctx := WithAntsHTTPClient(context.Background(), srv.Client(), srv.URL)
			_, err := ResolveAntsWorkUnit(ctx, "WU99999999999999999")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("not found"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("404"))
		})

		t.Run("403 Forbidden permission denied error handling", func(t *ftt.Test) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{
						"error": {
							"code": 403,
							"message": "The caller does not have permission",
							"status": "PERMISSION_DENIED"
						}
					}`))
			}))
			defer srv.Close()

			ctx := WithAntsHTTPClient(context.Background(), srv.Client(), srv.URL)
			_, err := ResolveAntsWorkUnit(ctx, "WU50500270009982574")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("permission denied"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("403"))
		})

		t.Run("Invalid AnTS work unit ID format", func(t *ftt.Test) {
			_, err := ResolveAntsWorkUnit(context.Background(), "not-a-wu-id")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("invalid AnTS work unit ID"))
		})
	})
}

func TestParseAntsWorkUnitJSON(t *testing.T) {
	ftt.Run("ParseAntsWorkUnitJSON", t, func(t *ftt.Test) {
		t.Run("Parses timing timestamps and properties", func(t *ftt.Test) {
			jsonData := []byte(`{
				"id": "WU89700270010105897",
				"invocationId": "I71400010624864031",
				"parentId": "WU97500270010256987",
				"name": "x86_64 bluetooth_test_gd_unit",
				"type": "TF_TEST_RUN",
				"state": "ERROR",
				"timing": {
					"creationTimestamp": "1789460743760",
					"completeTimestamp": "1789460782492"
				},
				"properties": [
					{"name": "attempt", "value": "0"},
					{"name": "resultdb_work_unit_id", "value": "ants-wu50500270009982574"}
				]
			}`)

			info, err := ParseAntsWorkUnitJSON(jsonData)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info.WorkUnitID, should.Equal("WU89700270010105897"))
			assert.Loosely(t, info.CreationTimestamp, should.Equal(int64(1789460743760)))
			assert.Loosely(t, info.CompleteTimestamp, should.Equal(int64(1789460782492)))
			assert.Loosely(t, info.State, should.Equal("ERROR"))
			assert.Loosely(t, info.ResultDBWorkUnitID, should.Equal("ants-wu50500270009982574"))
		})
	})
}

func TestResolveResultDBAntsWorkUnit(t *testing.T) {
	ftt.Run("ResolveResultDBAntsWorkUnit", t, func(t *ftt.Test) {
		ctx := context.Background()

		const (
			taskWUName = "rootInvocations/ants-i71400010624864031/workUnits/ants-wu50500270009982574"
			modWUName  = "rootInvocations/ants-i71400010624864031/workUnits/ants-wu50500270009982574:u-mod-1"
		)

		// newTaskAndModuleMockClient constructs a mock ResultDB client serving a
		// task work unit (`ants-wu50500270009982574`), its child `TF_MODULE` work
		// unit (`:u-mod-1`), and optional `TF_TEST_RUN` child work units.
		newTaskAndModuleMockClient := func(testRunChildNames []string, testRunWUs []*rdbpb.WorkUnit) *mockTargetsResultDBClient {
			return &mockTargetsResultDBClient{
				getWorkUnit: func(ctx context.Context, in *rdbpb.GetWorkUnitRequest, opts ...grpc.CallOption) (*rdbpb.WorkUnit, error) {
					if in.Name == taskWUName {
						return &rdbpb.WorkUnit{
							Name:           taskWUName,
							WorkUnitId:     "ants-wu50500270009982574",
							ChildWorkUnits: []string{modWUName},
						}, nil
					}
					return nil, status.Error(codes.NotFound, "not found")
				},
				batchGetWorkUnits: func(ctx context.Context, in *rdbpb.BatchGetWorkUnitsRequest, opts ...grpc.CallOption) (*rdbpb.BatchGetWorkUnitsResponse, error) {
					if len(in.Names) == 1 && in.Names[0] == modWUName {
						return &rdbpb.BatchGetWorkUnitsResponse{
							WorkUnits: []*rdbpb.WorkUnit{
								{
									Name:       modWUName,
									WorkUnitId: "ants-wu50500270009982574:u-mod-1",
									Kind:       "TF_MODULE",
									ModuleId: &rdbpb.ModuleIdentifier{
										ModuleName: "bluetooth_test_gd_unit",
										ModuleVariant: &rdbpb.Variant{
											Def: map[string]string{"module_abi": "x86_64"},
										},
									},
									ChildWorkUnits: testRunChildNames,
								},
							},
						}, nil
					}
					if len(testRunWUs) > 0 && len(in.Names) == len(testRunWUs) {
						return &rdbpb.BatchGetWorkUnitsResponse{WorkUnits: testRunWUs}, nil
					}
					return nil, status.Error(codes.NotFound, "not found")
				},
			}
		}

		t.Run("Already composite or attempt work unit returns as-is", func(t *ftt.Test) {
			res1 := ResolveResultDBAntsWorkUnit(ctx, nil, "ants-i123", "ants-wu50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64", nil)
			assert.Loosely(t, res1, should.Equal("ants-wu50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64"))

			res2 := ResolveResultDBAntsWorkUnit(ctx, nil, "ants-i123", "ants-wu123-attempt__0", nil)
			assert.Loosely(t, res2, should.Equal("ants-wu123-attempt__0"))
		})

		t.Run("Direct work unit found in ResultDB returns normWU", func(t *ftt.Test) {
			client := &mockTargetsResultDBClient{
				getWorkUnit: func(ctx context.Context, in *rdbpb.GetWorkUnitRequest, opts ...grpc.CallOption) (*rdbpb.WorkUnit, error) {
					if in.Name == taskWUName {
						return &rdbpb.WorkUnit{Name: in.Name, WorkUnitId: "ants-wu50500270009982574"}, nil
					}
					return nil, status.Error(codes.NotFound, "not found")
				},
			}

			res := ResolveResultDBAntsWorkUnit(ctx, client, "I71400010624864031", "WU50500270009982574", &AntsWorkUnitInfo{
				Type: "TFC_COMMAND_TASK",
			})
			assert.Loosely(t, res, should.Equal("ants-wu50500270009982574"))
		})

		t.Run("Top-level AnTS invocation work unit maps to root", func(t *ftt.Test) {
			client := &mockTargetsResultDBClient{
				getWorkUnit: func(ctx context.Context, in *rdbpb.GetWorkUnitRequest, opts ...grpc.CallOption) (*rdbpb.WorkUnit, error) {
					if in.Name == "rootInvocations/ants-i71400010624864031/workUnits/root" {
						return &rdbpb.WorkUnit{Name: in.Name, WorkUnitId: "root"}, nil
					}
					return nil, status.Error(codes.NotFound, "not found")
				},
			}

			res := ResolveResultDBAntsWorkUnit(ctx, client, "I71400010624864031", "WU98400270010026294", &AntsWorkUnitInfo{
				ParentID: "",
			})
			assert.Loosely(t, res, should.Equal("root"))
		})

		t.Run("AnTS leaf work unit resolves to composite child work unit via task and module batch get", func(t *ftt.Test) {
			testRunWUName := "rootInvocations/ants-i71400010624864031/workUnits/ants-wu50500270009982574:u-run-1"
			client := newTaskAndModuleMockClient([]string{testRunWUName}, nil)

			info := &AntsWorkUnitInfo{
				Type:               "TF_TEST_RUN",
				ModuleName:         "bluetooth_test_gd_unit",
				ModuleParameters:   []AntsProperty{{Name: "module-abi", Value: "x86_64"}},
				ResultDBWorkUnitID: "ants-wu50500270009982574",
			}

			res := ResolveResultDBAntsWorkUnit(ctx, client, "I71400010624864031", "WU89700270010105897", info)
			assert.Loosely(t, res, should.Equal("ants-wu50500270009982574:u-run-1"))
		})

		t.Run("AnTS TF_MODULE work unit resolves to module composite work unit", func(t *ftt.Test) {
			testRunWUName := "rootInvocations/ants-i71400010624864031/workUnits/ants-wu50500270009982574:u-run-1"
			client := newTaskAndModuleMockClient([]string{testRunWUName}, nil)

			info := &AntsWorkUnitInfo{
				Type:               "TF_MODULE",
				ModuleName:         "bluetooth_test_gd_unit",
				ResultDBWorkUnitID: "ants-wu50500270009982574",
			}

			res := ResolveResultDBAntsWorkUnit(ctx, client, "I71400010624864031", "WU97500270010256987", info)
			assert.Loosely(t, res, should.Equal("ants-wu50500270009982574:u-mod-1"))
		})

		t.Run("Multiple test run children: selects failed child via WorkUnit_FAILED state", func(t *ftt.Test) {
			run1Name := "rootInvocations/ants-i71400010624864031/workUnits/ants-wu50500270009982574:u-pass"
			run2Name := "rootInvocations/ants-i71400010624864031/workUnits/ants-wu50500270009982574:u-fail"

			client := newTaskAndModuleMockClient(
				[]string{run1Name, run2Name},
				[]*rdbpb.WorkUnit{
					{Name: run1Name, WorkUnitId: "ants-wu50500270009982574:u-pass", Kind: "TF_TEST_RUN", State: rdbpb.WorkUnit_SUCCEEDED},
					{Name: run2Name, WorkUnitId: "ants-wu50500270009982574:u-fail", Kind: "TF_TEST_RUN", State: rdbpb.WorkUnit_FAILED},
				},
			)

			info := &AntsWorkUnitInfo{
				Type:               "TF_TEST_RUN",
				State:              "ERROR",
				ModuleName:         "bluetooth_test_gd_unit",
				ResultDBWorkUnitID: "ants-wu50500270009982574",
			}

			res := ResolveResultDBAntsWorkUnit(ctx, client, "I71400010624864031", "WU89700270010105897", info)
			assert.Loosely(t, res, should.Equal("ants-wu50500270009982574:u-fail"))
		})

		t.Run("Multiple test run children: selects failed child via QueryTestVerdicts when both are SUCCEEDED", func(t *ftt.Test) {
			run1Name := "rootInvocations/ants-i71400010624864031/workUnits/ants-wu50500270009982574:u-pass"
			run2Name := "rootInvocations/ants-i71400010624864031/workUnits/ants-wu50500270009982574:u-fail"

			client := newTaskAndModuleMockClient(
				[]string{run1Name, run2Name},
				[]*rdbpb.WorkUnit{
					{Name: run1Name, WorkUnitId: "ants-wu50500270009982574:u-pass", Kind: "TF_TEST_RUN", State: rdbpb.WorkUnit_SUCCEEDED},
					{Name: run2Name, WorkUnitId: "ants-wu50500270009982574:u-fail", Kind: "TF_TEST_RUN", State: rdbpb.WorkUnit_SUCCEEDED},
				},
			)
			client.queryTestVerdicts = func(ctx context.Context, in *rdbpb.QueryTestVerdictsRequest, opts ...grpc.CallOption) (*rdbpb.QueryTestVerdictsResponse, error) {
				return &rdbpb.QueryTestVerdictsResponse{
					TestVerdicts: []*rdbpb.TestVerdict{
						{
							TestId: "://bluetooth_test_gd_unit!::Class#test1",
							Results: []*rdbpb.TestResult{
								{
									Name:     run1Name + "/tests/t1/results/r1",
									Expected: true,
									StatusV2: rdbpb.TestResult_PASSED,
								},
								{
									Name:     run2Name + "/tests/t1/results/r2",
									Expected: false,
									StatusV2: rdbpb.TestResult_FAILED,
								},
							},
						},
					},
				}, nil
			}

			info := &AntsWorkUnitInfo{
				Type:               "TF_TEST_RUN",
				State:              "ERROR",
				ModuleName:         "bluetooth_test_gd_unit",
				ResultDBWorkUnitID: "ants-wu50500270009982574",
			}

			res := ResolveResultDBAntsWorkUnit(ctx, client, "I71400010624864031", "WU89700270010105897", info)
			assert.Loosely(t, res, should.Equal("ants-wu50500270009982574:u-fail"))
		})

		t.Run("Multiple test run children: selects child via closest creation timestamp", func(t *ftt.Test) {
			run1Name := "rootInvocations/ants-i71400010624864031/workUnits/ants-wu50500270009982574:u-run-1"
			run2Name := "rootInvocations/ants-i71400010624864031/workUnits/ants-wu50500270009982574:u-run-2"

			t1 := timestamppb.New(time.UnixMilli(1700000000000))
			t2 := timestamppb.New(time.UnixMilli(1700000500000))

			client := newTaskAndModuleMockClient(
				[]string{run1Name, run2Name},
				[]*rdbpb.WorkUnit{
					{Name: run1Name, WorkUnitId: "ants-wu50500270009982574:u-run-1", Kind: "TF_TEST_RUN", State: rdbpb.WorkUnit_SUCCEEDED, CreateTime: t1},
					{Name: run2Name, WorkUnitId: "ants-wu50500270009982574:u-run-2", Kind: "TF_TEST_RUN", State: rdbpb.WorkUnit_SUCCEEDED, CreateTime: t2},
				},
			)

			info := &AntsWorkUnitInfo{
				Type:               "TF_TEST_RUN",
				ModuleName:         "bluetooth_test_gd_unit",
				CreationTimestamp:  1700000490000, // much closer to run2 (1700000500000) than run1
				ResultDBWorkUnitID: "ants-wu50500270009982574",
			}

			res := ResolveResultDBAntsWorkUnit(ctx, client, "I71400010624864031", "WU89700270010105897", info)
			assert.Loosely(t, res, should.Equal("ants-wu50500270009982574:u-run-2"))
		})
	})
}
