// Copyright 2026 The LUCI Authors.
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

package spanutil

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPageSizeController(t *testing.T) {
	ftt.Run("PageSizeController", t, func(t *ftt.Test) {
		opts := BufferingOptions{
			FirstPageSize:  10,
			SecondPageSize: 20,
			GrowthFactor:   1.0,
		}
		t.Run("Invalid Options", func(t *ftt.Test) {
			t.Run("FirstPageSize <= 0", func(t *ftt.Test) {
				opts.FirstPageSize = 0
				c := NewPageSizeController(opts)
				_, err := c.NextPageSize()
				assert.Loosely(t, err, should.ErrLike("initial buffer size must be positive"))
			})
			t.Run("SecondPageSize <= 0", func(t *ftt.Test) {
				opts.SecondPageSize = 0
				c := NewPageSizeController(opts)
				_, err := c.NextPageSize()
				assert.Loosely(t, err, should.ErrLike("second buffer size must be positive"))
			})
			t.Run("GrowthFactor < 1.0", func(t *ftt.Test) {
				opts.GrowthFactor = 0.9
				c := NewPageSizeController(opts)
				_, err := c.NextPageSize()
				assert.Loosely(t, err, should.ErrLike("growth factor must be equal or greater than 1.0"))
			})
		})

		t.Run("Progression", func(t *ftt.Test) {
			cases := []struct {
				name         string
				growthFactor float64
				expected     []int
			}{
				{
					name:         "GrowthFactor 1.0",
					growthFactor: 1.0,
					expected:     []int{10, 20, 20, 20, 20},
				},
				{
					name:         "GrowthFactor 1.5",
					growthFactor: 1.5,
					expected:     []int{10, 20, 30, 45, 67},
				},
				{
					name:         "GrowthFactor 2.0",
					growthFactor: 2.0,
					expected:     []int{10, 20, 40, 80, 160},
				},
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *ftt.Test) {
					opts.GrowthFactor = tc.growthFactor
					c := NewPageSizeController(opts)
					for _, expectedSize := range tc.expected {
						size, err := c.NextPageSize()
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, size, should.Equal(expectedSize))
					}
				})
			}
		})

		t.Run("MaxPageSize Cap", func(t *ftt.Test) {
			opts.FirstPageSize = 3010
			opts.SecondPageSize = 1000
			opts.GrowthFactor = 2.0
			c := NewPageSizeController(opts)

			expected := []int{3010, 1000, 2000, 4000, 8000, 10_000, 10_000, 10_000}

			for _, expectedSize := range expected {
				size, err := c.NextPageSize()
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, size, should.Equal(expectedSize))
			}
		})
	})
}
