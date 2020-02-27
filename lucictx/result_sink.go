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

package lucictx

import (
	"context"
	"fmt"
)

// ResultSink is a struct that may be used with the "result_sink" section of
// LUCI_CONTEXT.
type ResultSink struct {
	// TCP address (e.g. "localhost:62115") where a ResultSink pRPC server is hosted.
	Address string `json:"address"`
	// secret string required in all ResultSink requests in HTTP header
	// `Authorization: ResultSink <auth-token>`
	AuthToken string `json:"auth_token"`
}

// GetResultSink returns the current ResultSink from LUCI_CONTEXT if it was present.
// nil, otherwise.
func GetResultSink(ctx context.Context) *ResultSink {
	ret := ResultSink{}
	ok, err := Lookup(ctx, "result_sink", &ret)
	if err != nil {
		panic(err)
	}
	if !ok {
		return nil
	}
	return &ret
}

// SetResultSink sets the ResultSink in the LUCI_CONTEXT.
func SetResultSink(ctx context.Context, sink *ResultSink) context.Context {
	var raw interface{}
	if sink != nil {
		raw = sink
	}
	ctx, err := Set(ctx, "result_sink", raw)
	if err != nil {
		panic(fmt.Errorf("impossible: %s", err))
	}
	return ctx
}
