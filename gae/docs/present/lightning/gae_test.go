// Copyright 2016 The LUCI Authors.
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

//go:build !native_appengine
// +build !native_appengine

package demo

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

// START OMIT

// HL

func TestGAE(t *testing.T) {
	type Model struct { // HL
		ID   string `gae:"$id"` // HL
		A, B int    // HL
	} // HL
	ftt.Run("Put/Get w/ gae", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		assert.Loosely(t, datastore.Put(ctx, // HL
			&Model{"one thing", 10, 20},                 // HL
			&Model{"or another", 20, 30}), should.BeNil) // HL
		ms := []*Model{{ID: "one thing"}, {ID: "or another"}}
		assert.Loosely(t, datastore.Get(ctx, ms), should.BeNil) // HL
		assert.Loosely(t, ms, should.Resemble([]*Model{{"one thing", 10, 20}, {"or another", 20, 30}}))
	})
}

// END OMIT
