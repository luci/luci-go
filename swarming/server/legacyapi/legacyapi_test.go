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

package legacyapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/router"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
)

type fakeRequest struct {
	name string
}

type fakeResponse struct {
	greeting string
	bad      bool
}

func convertRequest(r *http.Request) (*fakeRequest, error) {
	name := r.Form.Get("name")
	if name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name required")
	}
	return &fakeRequest{name: name}, nil
}

func convertResponse(r *fakeResponse) (any, error) {
	if r.bad {
		return nil, status.Errorf(codes.Internal, "boom")
	}
	return map[string]string{"greeting": r.greeting}, nil
}

func fakeHandler(_ context.Context, req *fakeRequest) (*fakeResponse, error) {
	if req.name == "bad" {
		return nil, status.Errorf(codes.PermissionDenied, "bad name")
	}
	return &fakeResponse{
		greeting: "hi " + req.name,
		bad:      req.name == "boom",
	}, nil
}

func TestAdapt(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		params   string
		wantCode int
		wantBody string
	}{
		{
			name:     "ok",
			params:   "name=bob",
			wantCode: http.StatusOK,
			wantBody: "{\n  \"greeting\": \"hi bob\"\n}\n",
		},
		{
			name:     "bad URL form",
			params:   "&;",
			wantCode: http.StatusBadRequest,
			wantBody: "{\n  \"error\": {\n    \"message\": \"bad URL values in the request\"\n  }\n}\n",
		},
		{
			name:     "request parse error",
			params:   "something_else=1",
			wantCode: http.StatusBadRequest,
			wantBody: "{\n  \"error\": {\n    \"message\": \"name required\"\n  }\n}\n",
		},
		{
			name:     "handler error",
			params:   "name=bad",
			wantCode: http.StatusForbidden,
			wantBody: "{\n  \"error\": {\n    \"message\": \"bad name\"\n  }\n}\n",
		},
		{
			name:     "response serialize error",
			params:   "name=boom",
			wantCode: http.StatusInternalServerError,
			wantBody: "{\n  \"error\": {\n    \"message\": \"boom\"\n  }\n}\n",
		},
	}

	adapted := Adapt(convertRequest, convertResponse, fakeHandler)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rw := httptest.NewRecorder()
			adapted(&router.Context{
				Request: httptest.NewRequest("GET", "/path?"+tc.params, nil),
				Writer:  rw,
			})
			resp := rw.Result()
			assert.That(t, resp.StatusCode, should.Equal(tc.wantCode))
			assert.That(t, resp.Header.Get("Content-Type"), should.Equal("application/json; charset=utf-8"))
			body, _ := io.ReadAll(resp.Body)
			assert.That(t, string(body), should.Equal(tc.wantBody))
		})
	}
}

func TestRequestParsing(t *testing.T) {
	t.Parallel()

	parseDims := func(r *http.Request, p string) (any, error) { return Dimensions(r, p) }
	parseInt := func(r *http.Request, p string) (any, error) { return Int(r, p) }
	parseString := func(r *http.Request, p string) (any, error) { return String(r, p) }
	parseNullableBool := func(r *http.Request, p string) (any, error) { return NullableBool(r, p) }

	testCases := []struct {
		name    string
		params  string
		parser  func(req *http.Request, param string) (any, error)
		wantVal any
		wantErr any
	}{
		{
			name:   "dims ok",
			params: "param=k:v1&param=k%3Av2&param=a:b",
			parser: parseDims,
			wantVal: []*apipb.StringPair{
				{Key: "k", Value: "v1"},
				{Key: "k", Value: "v2"},
				{Key: "a", Value: "b"},
			},
		},
		{
			name:    "dims empty",
			params:  "something_else=1",
			parser:  parseDims,
			wantVal: []*apipb.StringPair(nil),
		},
		{
			name:    "dims bad",
			params:  "param=k",
			parser:  parseDims,
			wantErr: `not a valid dimension key-value pair "k"`,
		},
		{
			name:    "int ok",
			params:  "param=123",
			parser:  parseInt,
			wantVal: int64(123),
		},
		{
			name:    "int missing",
			params:  "something_else=1",
			parser:  parseInt,
			wantVal: int64(0),
		},
		{
			name:    "int bad",
			params:  "param=zzz",
			parser:  parseInt,
			wantErr: "not an integer",
		},
		{
			name:    "string ok",
			params:  "param=zzz",
			parser:  parseString,
			wantVal: "zzz",
		},
		{
			name:    "string missing",
			params:  "something_else=1",
			parser:  parseString,
			wantVal: "",
		},
		{
			name:    "string bad",
			params:  "param=zzz&param=yyy",
			parser:  parseString,
			wantErr: "has multiple values",
		},
		{
			name:    "bool NONE",
			params:  "param=NONE",
			parser:  parseNullableBool,
			wantVal: apipb.NullableBool_NULL,
		},
		{
			name:    "bool FALSE",
			params:  "param=FALSE",
			parser:  parseNullableBool,
			wantVal: apipb.NullableBool_FALSE,
		},
		{
			name:    "bool TRUE",
			params:  "param=TRUE",
			parser:  parseNullableBool,
			wantVal: apipb.NullableBool_TRUE,
		},
		{
			name:    "bool missing",
			params:  "something_else=1",
			parser:  parseNullableBool,
			wantVal: apipb.NullableBool_NULL,
		},
		{
			name:    "bool bad",
			params:  "param=huh",
			parser:  parseNullableBool,
			wantErr: "must be one of NONE, FALSE, TRUE",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/path?"+tc.params, nil)
			assert.That(t, req.ParseForm(), should.ErrLike(nil))
			got, err := tc.parser(req, "param")
			assert.That(t, err, should.ErrLike(tc.wantErr))
			if err == nil {
				assert.That(t, got, should.Match(tc.wantVal))
			}
		})
	}
}

func TestToTime(t *testing.T) {
	t.Parallel()

	assert.That(t, "2025-08-28T02:18:45.123456", should.Equal(ToTime(timestamppb.New(time.Unix(1756347525, 123456789)))))
}
