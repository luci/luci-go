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

package datastore

import (
	"bytes"
	"context"
	"strconv"
	"testing"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/data/cmpbin"

	"go.chromium.org/luci/gae/service/info"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestDatastoreQueryIterator(t *testing.T) {
	t.Parallel()

	ftt.Run("queryIterator", t, func(t *ftt.Test) {
		t.Run("normal", func(t *ftt.Test) {
			qi := queryIterator{
				order: []IndexColumn{
					{Property: "field1", Descending: true},
					{Property: "field2"},
					{Property: "__key__"},
				},
				itemCh: make(chan *rawQueryResult),
			}

			key := MkKeyContext("s~aid", "ns").MakeKey("testKind", 1)
			// populating results to the pipeline
			go func() {
				qi.itemCh <- &rawQueryResult{
					key: key,
					data: PropertyMap{
						"field1": PropertySlice{
							MkProperty("1"),
							MkProperty("11"),
						},
						"field2": MkProperty("aa1"),
					},
				}
				qi.itemCh <- &rawQueryResult{}
			}()
			done, err := qi.Next()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, done, should.BeFalse)
			assert.Loosely(t, qi.currentQueryResult, should.NotBeNil)

			t.Run("CurrentItemKey", func(t *ftt.Test) {
				itemKey := qi.CurrentItemKey()
				expectedKey := MkKeyContext("s~aid", "ns").MakeKey("testKind", 1)
				e := string(Serialize.ToBytes(expectedKey))
				assert.Loosely(t, itemKey, should.Equal(e))
			})

			t.Run("CurrentItemOrder", func(t *ftt.Test) {
				itemOrder := qi.CurrentItemOrder()
				assert.Loosely(t, err, should.BeNil)

				invBuf := cmpbin.Invertible(&bytes.Buffer{})
				invBuf.SetInvert(true)
				Serialize.Property(invBuf, MkProperty(strconv.Itoa(11)))
				invBuf.SetInvert(false)
				Serialize.Property(invBuf, MkProperty("aa1"))
				Serialize.Key(invBuf, key)

				assert.Loosely(t, itemOrder, should.Equal(invBuf.String()))
			})

			t.Run("CurrentItem", func(t *ftt.Test) {
				key, data := qi.CurrentItem()
				expectedPM := PropertyMap{
					"field1": PropertySlice{
						MkProperty("1"),
						MkProperty("11"),
					},
					"field2": MkProperty("aa1"),
				}
				assert.Loosely(t, key, should.Resemble(key))
				assert.Loosely(t, data, should.Resemble(expectedPM))
			})

			// end of results
			done, err = qi.Next()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, done, should.BeTrue)
		})

		t.Run("invalid queryIterator", func(t *ftt.Test) {
			qi := queryIterator{}
			assert.Loosely(t, func() { qi.Next() }, should.PanicLikeString(
				"item channel for queryIterator is not properly initiated"))
		})

		t.Run("empty query results", func(t *ftt.Test) {
			qi := &queryIterator{
				order:  []IndexColumn{},
				itemCh: make(chan *rawQueryResult),
			}
			go func() {
				qi.itemCh <- &rawQueryResult{
					key:  nil,
					data: PropertyMap{},
				}
			}()

			done, err := qi.Next()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, done, should.BeTrue)
			assert.Loosely(t, qi.CurrentItemKey(), should.BeEmpty)
			itemOrder := qi.CurrentItemOrder()
			assert.Loosely(t, itemOrder, should.BeEmpty)
			key, data := qi.CurrentItem()
			assert.Loosely(t, key, should.BeNil)
			assert.Loosely(t, data, should.Resemble(PropertyMap{}))
		})
	})
}

func TestStartQueryIterator(t *testing.T) {
	t.Parallel()

	ftt.Run("start queryIterator", t, func(t *ftt.Test) {
		ctx := info.Set(context.Background(), fakeInfo{})
		fds := fakeDatastore{}
		ctx = SetRawFactory(ctx, fds.factory())
		ctx, cancel := context.WithCancel(ctx)

		fds.entities = 2
		dq := NewQuery("Kind").Order("Value")

		eg, ectx := errgroup.WithContext(ctx)

		t.Run("found", func(t *ftt.Test) {
			fq, err := dq.Finalize()
			assert.Loosely(t, err, should.BeNil)
			qi := startQueryIterator(ectx, eg, fq)

			done, err := qi.Next()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, done, should.BeFalse)
			assert.Loosely(t, qi.currentQueryResult.key, should.Resemble(MakeKey(ctx, "Kind", 1)))
			assert.Loosely(t, qi.currentQueryResult.data, should.Resemble(
				PropertyMap{
					"Value": MkProperty(0),
				}))

			done, err = qi.Next()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, done, should.BeFalse)
			assert.Loosely(t, qi.currentQueryResult.key, should.Resemble(MakeKey(ctx, "Kind", 2)))
			assert.Loosely(t, qi.currentQueryResult.data, should.Resemble(
				PropertyMap{
					"Value": MkProperty(1),
				}))
			assert.Loosely(t, qi.currentItemOrderCache, should.BeEmpty)
			order := qi.CurrentItemOrder()
			assert.Loosely(t, qi.currentItemOrderCache, should.Equal(order))

			done, err = qi.Next()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, done, should.BeTrue)
		})

		t.Run("cancel", func(t *ftt.Test) {
			fq, err := dq.Finalize()
			assert.Loosely(t, err, should.BeNil)
			qi := startQueryIterator(ectx, eg, fq)

			cancel()
			<-ctx.Done() // wait till the cancellation propagates

			// This is to test the goroutine in startQueryIterator() is cancelled after the `cancel()`.
			// So it asserts two possible scenarios: 1) qi.Next() directly returns a Stop signal.
			// 2) qi.Next() retrieves at most one rawQueryResult and then returns a Stop signal.
			done, err := qi.Next()
			if !done {
				assert.Loosely(t, qi.currentQueryResult, should.Resemble(&rawQueryResult{
					key: MakeKey(ctx, "Kind", 1),
					data: PropertyMap{
						"Value": MkProperty(0),
					},
				}))
				done, err = qi.Next()
				assert.Loosely(t, err, should.Equal(context.Canceled))
				assert.Loosely(t, done, should.BeTrue)
			} else {
				assert.Loosely(t, err, should.Equal(context.Canceled))
				assert.Loosely(t, done, should.BeTrue)
			}
		})

		t.Run("not found", func(t *ftt.Test) {
			fds.entities = 0
			fq, err := dq.Finalize()
			assert.Loosely(t, err, should.BeNil)
			qi := startQueryIterator(ectx, eg, fq)

			done, err := qi.Next()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, done, should.BeTrue)
		})

		t.Run("errors from raw datastore", func(t *ftt.Test) {
			dq = dq.Eq("@err_single", "Query fail").Eq("@err_single_idx", 0)
			fq, err := dq.Finalize()
			assert.Loosely(t, err, should.BeNil)
			qi := startQueryIterator(ectx, eg, fq)

			done, err := qi.Next()
			assert.Loosely(t, err, should.ErrLike("Query fail"))
			assert.Loosely(t, done, should.BeTrue)
		})
	})
}
