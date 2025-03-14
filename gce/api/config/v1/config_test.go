// Copyright 2018 The LUCI Authors.
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

package config

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("ComputeAmount", t, func(t *ftt.Test) {
		cfg := &Config{
			Amount: &Amount{
				Min: 1,
				Max: 3,
			},
		}
		amt, err := cfg.ComputeAmount(2, time.Time{})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, amt, should.Equal(2))
	})

	ftt.Run("Validate", t, func(t *ftt.Test) {
		c := &validation.Context{Context: context.Background()}

		t.Run("invalid", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				cfg := &Config{}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("at least one disk is required"))
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("prefix is required"))
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("duration or seconds is required"))
			})

			t.Run("current amount", func(t *ftt.Test) {
				cfg := &Config{
					CurrentAmount: 1,
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("current amount must not be specified"))
			})

			t.Run("revision", func(t *ftt.Test) {
				cfg := &Config{
					Revision: "revision",
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("revision must not be specified"))
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			cfg := &Config{
				Attributes: &VM{
					Disk: []*Disk{
						{
							Image: "global/images/image",
						},
					},
					MachineType: "type",
					NetworkInterface: []*NetworkInterface{
						{},
					},
					Project: "project",
					Zone:    "zone",
				},
				Lifetime: &TimePeriod{
					Time: &TimePeriod_Seconds{
						Seconds: 3600,
					},
				},
				Prefix: "prefix",
			}
			cfg.Validate(c, true)
			assert.Loosely(t, c.Finalize(), should.BeNil)
		})
	})
}
