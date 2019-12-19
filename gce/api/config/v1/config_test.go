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

	"go.chromium.org/luci/config/validation"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	Convey("ComputeAmount", t, func() {
		cfg := &Config{
			Amount: &Amount{
				Min: 1,
				Max: 3,
			},
			CurrentAmount: 2,
		}
		amt, err := cfg.ComputeAmount(time.Time{})
		So(err, ShouldBeNil)
		So(amt, ShouldEqual, 2)
	})

	Convey("Validate", t, func() {
		c := &validation.Context{Context: context.Background()}

		Convey("invalid", func() {
			Convey("empty", func() {
				cfg := &Config{}
				cfg.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				So(errs, ShouldContainErr, "at least one disk is required")
				So(errs, ShouldContainErr, "prefix is required")
				So(errs, ShouldContainErr, "duration or seconds is required")
			})

			Convey("amount", func() {
				cfg := &Config{
					Amount: &Amount{
						Default: -1,
					},
				}
				cfg.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				So(errs, ShouldContainErr, "default amount must be non-negative")
			})

			Convey("current amount", func() {
				cfg := &Config{
					CurrentAmount: 1,
				}
				cfg.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				So(errs, ShouldContainErr, "current amount must not be specified")
			})

			Convey("revision", func() {
				cfg := &Config{
					Revision: "revision",
				}
				cfg.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				So(errs, ShouldContainErr, "revision must not be specified")
			})
		})

		Convey("valid", func() {
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
			cfg.Validate(c)
			So(c.Finalize(), ShouldBeNil)
		})
	})
}
