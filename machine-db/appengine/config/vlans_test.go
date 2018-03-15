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

package config

import (
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/machine-db/api/common/v1"
	"go.chromium.org/luci/machine-db/api/config/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateVLANs(t *testing.T) {
	t.Parallel()

	Convey("validateVLANs", t, func() {
		context := &validation.Context{Context: context.Background()}

		Convey("empty config", func() {
			vlans := &config.VLANs{}
			validateVLANs(context, vlans)
			So(context.Finalize(), ShouldBeNil)
		})

		Convey("missing VLAN ID", func() {
			vlans := &config.VLANs{
				Vlan: []*config.VLAN{
					{
						CidrBlock: "127.0.0.1/32",
					},
				},
			}
			validateVLANs(context, vlans)
			So(context.Finalize(), ShouldErrLike, "must be positive")
		})

		Convey("negative VLAN ID", func() {
			vlans := &config.VLANs{
				Vlan: []*config.VLAN{
					{
						Id:        -1,
						CidrBlock: "127.0.0.1/32",
					},
				},
			}
			validateVLANs(context, vlans)
			So(context.Finalize(), ShouldErrLike, "must be positive")
		})

		Convey("zero VLAN ID", func() {
			vlans := &config.VLANs{
				Vlan: []*config.VLAN{
					{
						Id:        0,
						CidrBlock: "127.0.0.1/32",
					},
				},
			}
			validateVLANs(context, vlans)
			So(context.Finalize(), ShouldErrLike, "must be positive")
		})

		Convey("excessive VLAN ID", func() {
			vlans := &config.VLANs{
				Vlan: []*config.VLAN{
					{
						Id:        65536,
						CidrBlock: "127.0.0.1/32",
					},
				},
			}
			validateVLANs(context, vlans)
			So(context.Finalize(), ShouldErrLike, "must not exceed 65535")
		})

		Convey("duplicate VLAN", func() {
			vlans := &config.VLANs{
				Vlan: []*config.VLAN{
					{
						Id:        1,
						CidrBlock: "127.0.0.1/32",
					},
					{
						Id:        2,
						CidrBlock: "127.0.0.1/32",
					},
					{
						Id:        1,
						CidrBlock: "127.0.0.1/32",
					},
				},
			}
			validateVLANs(context, vlans)
			So(context.Finalize(), ShouldErrLike, "duplicate VLAN")
		})

		Convey("invalid CIDR block", func() {
			vlans := &config.VLANs{
				Vlan: []*config.VLAN{
					{
						Id:        1,
						CidrBlock: "512.0.0.1/128",
					},
				},
			}
			validateVLANs(context, vlans)
			So(context.Finalize(), ShouldErrLike, "invalid CIDR block")
		})

		Convey("CIDR block too long", func() {
			vlans := &config.VLANs{
				Vlan: []*config.VLAN{
					{
						Id:        1,
						CidrBlock: "127.0.0.1/1",
					},
				},
			}
			validateVLANs(context, vlans)
			So(context.Finalize(), ShouldErrLike, "CIDR block suffix must be at least 18")
		})

		Convey("ok", func() {
			vlans := &config.VLANs{
				Vlan: []*config.VLAN{
					{
						Id:        1,
						CidrBlock: "127.0.0.1/32",
						State:     common.State_SERVING,
					},
					{
						Id:        2,
						CidrBlock: "127.0.0.1/32",
						State:     common.State_PRERELEASE,
					},
				},
			}
			validateVLANs(context, vlans)
			So(context.Finalize(), ShouldBeNil)
		})
	})
}
