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

// Package legacyapi implements an adapter between the legacy and gRPC APIs.
//
// Supports only GET endpoints returning JSON data, since this is the only kind
// of legacy endpoints that are still in use as of Aug 2025.
//
// See cmd/default/main.go where legacy endpoints are actually installed.
package legacyapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
)

// Adapt adapts a gRPC request handler to look like a legacy API handler.
//
// It returns a legacy handler that deserializes the request using `request`
// callback, then calls `handler` callback, and serializes the response using
// `response` callback.
func Adapt[Req any, Resp any](
	request func(*http.Request) (*Req, error), // raw HTTP request => proto message
	response func(*Resp) (any, error), // proto message => JSON-serializable response
	handler func(_ context.Context, req *Req) (*Resp, error), // the actual handler
) func(ctx *router.Context) {
	return func(ctx *router.Context) {
		if err := ctx.Request.ParseForm(); err != nil {
			writeErr(ctx, status.Errorf(codes.InvalidArgument, "bad URL values in the request"))
			return
		}
		req, err := request(ctx.Request)
		if err != nil {
			writeErr(ctx, err)
			return
		}
		logging.Infof(ctx.Request.Context(), "Legacy API call from %q: %s",
			auth.CurrentIdentity(ctx.Request.Context()), req)
		res, err := handler(ctx.Request.Context(), req)
		if err != nil {
			writeErr(ctx, err)
			return
		}
		body, err := response(res)
		if err != nil {
			writeErr(ctx, err)
			return
		}
		blob, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			writeErr(ctx, err)
			return
		}
		blob = append(blob, '\n')
		ctx.Writer.Header().Add("Content-Type", "application/json; charset=utf-8")
		ctx.Writer.WriteHeader(http.StatusOK)
		_, _ = ctx.Writer.Write(blob)
	}
}

func writeErr(ctx *router.Context, err error) {
	st, _ := status.FromError(err)
	ctx.Writer.Header().Add("Content-Type", "application/json; charset=utf-8")
	ctx.Writer.WriteHeader(grpcutil.CodeStatus(st.Code()))
	var resp struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	resp.Error.Message = st.Message()
	blob, _ := json.MarshalIndent(&resp, "", "  ")
	blob = append(blob, '\n')
	_, _ = ctx.Writer.Write(blob)
}

// Dimensions extracts "dimensions" request parameter.
//
// Doesn't do any validation beyond just parsing the request parameter.
func Dimensions(req *http.Request, param string) ([]*apipb.StringPair, error) {
	var dims []*apipb.StringPair
	for _, kv := range req.Form[param] {
		k, v, found := strings.Cut(kv, ":")
		if !found {
			return nil, status.Errorf(codes.InvalidArgument, "not a valid dimension key-value pair %q", kv)
		}
		dims = append(dims, &apipb.StringPair{Key: k, Value: v})
	}
	return dims, nil
}

// Int extracts an integer request parameter.
func Int(req *http.Request, param string) (int64, error) {
	asStr, err := String(req, param)
	if err != nil || asStr == "" {
		return 0, err
	}
	val, err := strconv.ParseInt(asStr, 10, 64)
	if err != nil {
		return 0, status.Errorf(codes.InvalidArgument, "parameter %q is not an integer: %s", param, err)
	}
	return val, nil
}

// String extracts a string request parameter.
func String(req *http.Request, param string) (string, error) {
	switch v := req.Form[param]; {
	case len(v) == 0:
		return "", nil
	case len(v) == 1:
		return v[0], nil
	default:
		return "", status.Errorf(codes.InvalidArgument, "parameter %q has multiple values", param)
	}
}

// NullableBool extracts a nullable bool request parameter.
func NullableBool(req *http.Request, param string) (apipb.NullableBool, error) {
	switch asStr, err := String(req, param); {
	case err != nil:
		return 0, err
	case asStr == "" || asStr == "NONE": // yeah, it is NONE, not NULL
		return apipb.NullableBool_NULL, nil
	case asStr == "FALSE":
		return apipb.NullableBool_FALSE, nil
	case asStr == "TRUE":
		return apipb.NullableBool_TRUE, nil
	default:
		return 0, status.Errorf(codes.InvalidArgument, "parameter %q must be one of NONE, FALSE, TRUE, got %q", param, asStr)
	}
}

// ToTime converts a protobuf timestamp to a legacy timestamp field value.
func ToTime(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return ""
	}
	return ts.AsTime().Format("2006-01-02T15:04:05.999999")
}
