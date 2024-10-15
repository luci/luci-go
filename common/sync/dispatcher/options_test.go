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

package dispatcher

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/time/rate"

	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestOptionValidationGood(t *testing.T) {
	fullOptions := Options[string]{
		ErrorFn:  func(*buffer.Batch[string], error) bool { return false },
		DropFn:   DropFnQuiet[string],
		QPSLimit: rate.NewLimiter(rate.Inf, 0),
		Buffer:   buffer.Defaults,
	}

	var goodOptions = []struct {
		name     string
		options  Options[string]
		expected Options[string]
	}{
		{
			name: "minimal",
			options: Options[string]{
				Buffer: buffer.Defaults,
			},
			expected: Options[string]{
				QPSLimit: rate.NewLimiter(rate.Inf, 0),
				Buffer:   buffer.Defaults,
			},
		},

		{
			name:     "full",
			options:  fullOptions,
			expected: fullOptions,
		},
	}

	funcCheck := cmp.FilterPath(
		func(p cmp.Path) bool {
			return p.Last().Type().Kind() == reflect.Func
		}, cmp.Transformer("func.pointer", func(fn any) uintptr {
			if fn == nil {
				return 0
			}
			return reflect.ValueOf(fn).Pointer()
		}))

	ftt.Run(`test good option groups`, t, func(t *ftt.Test) {
		ctx := context.Background()
		for _, options := range goodOptions {
			t.Run(options.name, func(t *ftt.Test) {
				myOptions := options.options
				expect := options.expected

				// ShouldResemble has issues with function pointers; don't care about
				// testing buffer options in this test.
				myOptions.Buffer.Retry = nil
				expect.Buffer.Retry = nil

				assert.Loosely(t, myOptions.normalize(ctx), should.BeNil)

				if expect.ErrorFn == nil {
					assert.Loosely(t, myOptions.ErrorFn, should.NotBeNil) // default is non-nil
				} else {
					assert.That(t, myOptions.ErrorFn, should.Match(expect.ErrorFn, funcCheck))
					expect.ErrorFn = nil
				}
				myOptions.ErrorFn = nil

				if expect.DropFn == nil {
					assert.Loosely(t, myOptions.DropFn, should.NotBeNil) // default is non-nil
				} else {
					assert.That(t, &myOptions.DropFn, should.Match(&expect.DropFn, funcCheck))
					expect.DropFn = nil
				}
				myOptions.DropFn = nil

				if expect.ItemSizeFunc == nil {
					assert.Loosely(t, myOptions.ItemSizeFunc, should.BeNil)
				} else {
					assert.Loosely(t, &myOptions.ItemSizeFunc, should.Match(&expect.ItemSizeFunc, funcCheck))
					expect.ItemSizeFunc = nil
				}
				myOptions.ItemSizeFunc = nil

				assert.Loosely(t, myOptions, should.Resemble(expect))
			})
		}
	})
}

func TestOptionValidationBad(t *testing.T) {
	ftt.Run(`bad option validation`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`QPSLimit`, func(t *ftt.Test) {
			opts := Options[string]{QPSLimit: rate.NewLimiter(100, 0), Buffer: buffer.Defaults}
			assert.Loosely(t, opts.normalize(ctx), should.ErrLike("QPSLimit has burst size < 1"))
		})

		t.Run(`ItemSizeFunc == nil`, func(t *ftt.Test) {
			t.Run(`BatchSizeMax > 0`, func(t *ftt.Test) {
				opts := Options[string]{Buffer: buffer.Defaults}
				opts.Buffer.BatchSizeMax = 1000
				assert.Loosely(t, opts.normalize(ctx), should.ErrLike("Buffer.BatchSizeMax > 0"))
			})
		})

		t.Run(`MinQPS`, func(t *ftt.Test) {
			t.Run(`MinQPS == rate.Inf`, func(t *ftt.Test) {
				opts := Options[string]{
					MinQPS: rate.Inf,
					Buffer: buffer.Defaults}
				assert.Loosely(t, opts.normalize(ctx), should.ErrLike("MinQPS cannot be infinite"))
			})
			t.Run(`MinQPS greater than QPSLimit`, func(t *ftt.Test) {
				opts := Options[string]{
					QPSLimit: rate.NewLimiter(100, 1),
					MinQPS:   rate.Every(time.Millisecond),
					Buffer:   buffer.Defaults}
				assert.Loosely(t, opts.normalize(ctx), should.ErrLike("MinQPS: 1000.000000 is greater than QPSLimit: 100.000000"))
			})
		})
	})
}
