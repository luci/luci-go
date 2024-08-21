// Copyright 2017 The LUCI Authors.
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

package upload

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
)

func TestOperation(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		testTime := testclock.TestRecentTimeUTC.Round(time.Millisecond)

		ctx := memory.Use(context.Background())
		ctx, _ = testclock.UseTime(ctx, testTime)

		t.Run("ToProto", func(t *ftt.Test) {
			op := Operation{
				ID:        123,
				Status:    api.UploadStatus_UPLOADING,
				UploadURL: "http://upload-url.example.com",
				HashAlgo:  api.HashAlgo_SHA256,
				HexDigest: strings.Repeat("a", 64),
				Error:     "zzz",
			}

			// No Object when in UPLOADING.
			assert.Loosely(t, op.ToProto("wrappedID"), should.Resemble(&api.UploadOperation{
				OperationId:  "wrappedID",
				UploadUrl:    "http://upload-url.example.com",
				Status:       api.UploadStatus_UPLOADING,
				ErrorMessage: "zzz",
			}))

			// With Object when in PUBLISHED.
			op.Status = api.UploadStatus_PUBLISHED
			assert.Loosely(t, op.ToProto("wrappedID"), should.Resemble(&api.UploadOperation{
				OperationId: "wrappedID",
				UploadUrl:   "http://upload-url.example.com",
				Status:      api.UploadStatus_PUBLISHED,
				Object: &api.ObjectRef{
					HashAlgo:  op.HashAlgo,
					HexDigest: op.HexDigest,
				},
				ErrorMessage: "zzz",
			}))
		})

		t.Run("Advance works", func(t *ftt.Test) {
			op := &Operation{
				ID:     123,
				Status: api.UploadStatus_UPLOADING,
			}

			t.Run("Success", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, op), should.BeNil)
				newOp, err := op.Advance(ctx, func(_ context.Context, o *Operation) error {
					o.Status = api.UploadStatus_ERRORED
					o.Error = "zzz"
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, newOp, should.Resemble(&Operation{
					ID:        123,
					Status:    api.UploadStatus_ERRORED,
					Error:     "zzz",
					UpdatedTS: testTime,
				}))

				// Original one untouched.
				assert.Loosely(t, op.Error, should.BeEmpty)

				// State in the datastore is really updated.
				assert.Loosely(t, datastore.Get(ctx, op), should.BeNil)
				assert.Loosely(t, op, should.Resemble(newOp))
			})

			t.Run("Skips because of unexpected status", func(t *ftt.Test) {
				cpy := *op
				cpy.Status = api.UploadStatus_ERRORED
				assert.Loosely(t, datastore.Put(ctx, &cpy), should.BeNil)

				newOp, err := op.Advance(ctx, func(context.Context, *Operation) error {
					panic("must not be called")
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, newOp, should.Resemble(&cpy))
			})

			t.Run("Callback error", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, op), should.BeNil)
				_, err := op.Advance(ctx, func(_ context.Context, o *Operation) error {
					o.Error = "zzz" // must be ignored
					return errors.New("omg")
				})
				assert.Loosely(t, err, should.ErrLike("omg"))
				assert.Loosely(t, datastore.Get(ctx, op), should.BeNil)
				assert.Loosely(t, op.Error, should.BeEmpty)
			})

			t.Run("Missing entity", func(t *ftt.Test) {
				_, err := op.Advance(ctx, func(context.Context, *Operation) error {
					panic("must not be called")
				})
				assert.Loosely(t, err, should.ErrLike("no such entity"))
			})
		})
	})
}
