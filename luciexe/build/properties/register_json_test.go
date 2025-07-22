// Copyright 2024 The LUCI Authors.
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

package properties

import (
	"context"
	"encoding/json"
	"testing"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestStructFromStruct(t *testing.T) {
	t.Parallel()

	type someStruct struct {
		ID int `json:"id"`
	}

	t.Run(`strict`, func(t *testing.T) {
		t.Parallel()

		target := &someStruct{}
		ctx, ml := mctx()

		badExtras, err := jsonFromStruct(false, true)(ctx, "", rejectUnknownFields, mustStruct(map[string]any{
			"id": 1234,
		}), target)
		assert.That(t, badExtras, should.BeFalse)
		assert.NoErr(t, err)
		assert.That(t, target, should.Match(&someStruct{
			ID: 1234,
		}))

		badExtras, err = jsonFromStruct(false, true)(ctx, "", rejectUnknownFields, mustStruct(map[string]any{
			"morple": 100,
			"id":     1234,
		}), target)
		assert.That(t, badExtras, should.BeTrue)
		assert.NoErr(t, err)
		assert.That(t, ml.Messages(), should.Match([]memlogger.LogEntry{
			{Level: logging.Error, Msg: `Unknown fields while parsing property namespace "": {"morple":100}`, CallDepth: 2},
		}))
	})

	t.Run(`ignore unknown`, func(t *testing.T) {
		t.Parallel()

		target := &someStruct{}

		ctx, ml := mctx()

		badExtras, err := jsonFromStruct(false, true)(ctx, "", logUnknownFields, mustStruct(map[string]any{
			"morple": 100,
			"id":     1234,
		}), target)
		assert.That(t, badExtras, should.BeFalse)
		assert.NoErr(t, err)
		assert.That(t, target, should.Match(&someStruct{
			ID: 1234,
		}))
		assert.That(t, ml.Messages(), should.Match([]memlogger.LogEntry{
			{Level: logging.Warning, Msg: `Unknown fields while parsing property namespace "": {"morple":100}`, CallDepth: 2},
		}))
	})

	t.Run(`maps don't check for overlap`, func(t *testing.T) {
		t.Parallel()

		target := map[string]any{}
		badExtras, err := jsonFromStruct(true, false)(context.Background(), "", logUnknownFields, mustStruct(map[string]any{
			"morple": 100,
			"id":     "hi",
		}), &target)
		assert.That(t, badExtras, should.BeFalse)
		assert.NoErr(t, err)
		assert.That(t, target, should.Match(map[string]any{
			"morple": json.Number("100"),
			"id":     "hi",
		}))
	})
}
