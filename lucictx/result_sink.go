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
)

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
	return Set(ctx, "result_sink", sink)
}
