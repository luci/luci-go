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

// ResultDB is a struct that may be used with the "resultDB" section of LUCI_CONTEXT.
type ResultDB struct {
	TestResults TestResults
}

// TestResults is a struct that may be used with the "resultDB.testResults" section of
// LUCI_CONTEXT.
type TestResults struct {
	Port      int
	AuthToken string
}

// GetResultDB returns the current ResultDB from LUCI_CONTEXT
// if it was present. If no ResultDB is in the context it returns nil.
func GetResultDB(ctx context.Context) *ResultDB {
	ret := ResultDB{}
	ok, err := Lookup(ctx, "resultDB", &ret)
	if err != nil {
		panic(err)
	}
	if !ok {
		return nil
	}
	return &ret
}

// SetResultDB sets the ResultDB in the LUCI_CONTEXT.
func SetResultDB(ctx context.Context, sink *ResultDB) context.Context {
	var raw interface{}
	if sink != nil {
		raw = sink
	}
	ctx, err := Set(ctx, "resultDB", raw)
	if err != nil {
		panic(fmt.Errorf("impossible: %s", err))
	}
	return ctx
}
