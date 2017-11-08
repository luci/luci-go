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

package database

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/machine-db/api/config/v1"

	. "go.chromium.org/luci/common/testing/assertions"

	"golang.org/x/net/context"
)

func TestValidateDatacenters(t *testing.T) {
	t.Parallel()

	Convey("validateDatacenters: empty datacenters config", t, func() {
		datacentersConfig := []*config.DatacenterConfig{}
		e := validateDatacenters(context.Background(), datacentersConfig)
		So(e, ShouldBeNil)
	})

	Convey("validateDatacenters: unnamed datacenter", t, func() {
		datacentersConfig := []*config.DatacenterConfig{
			{},
		}
		e := validateDatacenters(context.Background(), datacentersConfig)
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "Datacenter names are required and must be non-empty")
	})

	Convey("validateDatacenters: duplicate datacenters", t, func() {
		datacentersConfig := []*config.DatacenterConfig{
			{
				Name: "duplicate",
			},
			{
				Name: "unique",
			},
			{
				Name: "duplicate",
			},
		}
		e := validateDatacenters(context.Background(), datacentersConfig)
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "Duplicate datacenter: duplicate")
	})

	Convey("validateDatacenters: unnamed rack", t, func() {
		datacentersConfig := []*config.DatacenterConfig{
			{
				Name: "datacenter",
				Rack: []*config.RackConfig{
					{},
				},
			},
		}
		e := validateDatacenters(context.Background(), datacentersConfig)
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "Rack names are required and must be non-empty")
	})

	Convey("validateDatacenters: duplicate racks in the same datacenter", t, func() {
		datacentersConfig := []*config.DatacenterConfig{
			{
				Name: "datacenter",
				Rack: []*config.RackConfig{
					{
						Name: "duplicate",
					},
					{
						Name: "unique",
					},
					{
						Name: "duplicate",
					},
				},
			},
		}
		e := validateDatacenters(context.Background(), datacentersConfig)
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "Duplicate rack: duplicate")
	})

	Convey("validateDatacenters: duplicate racks in different datacenters", t, func() {
		datacentersConfig := []*config.DatacenterConfig{
			{
				Name: "datacenter 1",
				Rack: []*config.RackConfig{
					{
						Name: "duplicate",
					},
					{
						Name: "unique",
					},
				},
			},
			{
				Name: "datacenter 2",
				Rack: []*config.RackConfig{
					{
						Name: "duplicate",
					},
				},
			},
		}
		e := validateDatacenters(context.Background(), datacentersConfig)
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "Duplicate rack: duplicate")
	})

	Convey("validateDatacenters: unnamed switch", t, func() {
		datacentersConfig := []*config.DatacenterConfig{
			{
				Name: "datacenter",
				Rack: []*config.RackConfig{
					{
						Name: "rack",
						Switch: []*config.SwitchConfig{
							{},
						},
					},
				},
			},
		}
		e := validateDatacenters(context.Background(), datacentersConfig)
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "Switch names are required and must be non-empty")
	})

	Convey("validateDatacenters: duplicate switches in the same rack", t, func() {
		datacentersConfig := []*config.DatacenterConfig{
			{
				Name: "datacenter",
				Rack: []*config.RackConfig{
					{
						Name: "rack",
						Switch: []*config.SwitchConfig{
							{
								Name:  "duplicate",
								Ports: 2,
							},
							{
								Name:  "unique",
								Ports: 4,
							},
							{
								Name:  "duplicate",
								Ports: 8,
							},
						},
					},
				},
			},
		}
		e := validateDatacenters(context.Background(), datacentersConfig)
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "Duplicate switch: duplicate")
	})

	Convey("validateDatacenters: duplicate switches in different racks", t, func() {
		datacentersConfig := []*config.DatacenterConfig{
			{
				Name: "datacenter 1",
				Rack: []*config.RackConfig{
					{
						Name: "rack 1",
						Switch: []*config.SwitchConfig{
							{
								Name:  "duplicate",
								Ports: 2,
							},
							{
								Name:  "unique",
								Ports: 4,
							},
						},
					},
				},
			},
			{
				Name: "datacenter 2",
				Rack: []*config.RackConfig{
					{
						Name: "rack 2",
						Switch: []*config.SwitchConfig{
							{
								Name:  "duplicate",
								Ports: 8,
							},
						},
					},
				},
			},
		}
		e := validateDatacenters(context.Background(), datacentersConfig)
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "Duplicate switch: duplicate")
	})

	Convey("validateDatacenters: missing switch ports", t, func() {
		datacentersConfig := []*config.DatacenterConfig{
			{
				Name: "datacenter",
				Rack: []*config.RackConfig{
					{
						Name: "rack",
						Switch: []*config.SwitchConfig{
							{
								Name: "switch",
							},
						},
					},
				},
			},
		}
		e := validateDatacenters(context.Background(), datacentersConfig)
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "Switch must have at least one port: switch")
	})

	Convey("validateDatacenters: negative switch ports", t, func() {
		datacentersConfig := []*config.DatacenterConfig{
			{
				Name: "datacenter",
				Rack: []*config.RackConfig{
					{
						Name: "rack",
						Switch: []*config.SwitchConfig{
							{
								Name:  "switch",
								Ports: -1,
							},
						},
					},
				},
			},
		}
		e := validateDatacenters(context.Background(), datacentersConfig)
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "Switch must have at least one port: switch")
	})

	Convey("validateDatacenters: zero switch ports", t, func() {
		datacentersConfig := []*config.DatacenterConfig{
			{
				Name: "datacenter",
				Rack: []*config.RackConfig{
					{
						Name: "rack",
						Switch: []*config.SwitchConfig{
							{
								Name:  "switch",
								Ports: 0,
							},
						},
					},
				},
			},
		}
		e := validateDatacenters(context.Background(), datacentersConfig)
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "Switch must have at least one port: switch")
	})

	Convey("validateDatacenters: excessive switch ports", t, func() {
		datacentersConfig := []*config.DatacenterConfig{
			{
				Name: "datacenter",
				Rack: []*config.RackConfig{
					{
						Name: "rack",
						Switch: []*config.SwitchConfig{
							{
								Name:  "switch",
								Ports: 65536,
							},
						},
					},
				},
			},
		}
		e := validateDatacenters(context.Background(), datacentersConfig)
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "Switch must have at most 65535 ports: switch")
	})

	Convey("validateDatacenters: ok", t, func() {
		datacentersConfig := []*config.DatacenterConfig{
			{
				Name:        "datacenter 1",
				Description: "A description of datacenter 1",
				Rack: []*config.RackConfig{
					{
						Name:        "rack 1",
						Description: "A description of rack 1",
						Switch: []*config.SwitchConfig{
							{
								Name:        "switch 1",
								Description: "A description of switch 1",
								Ports:       4,
							},
						},
					},
					{
						Name: "rack 2",
					},
				},
			},
			{
				Name: "datacenter 2",
			},
			{
				Name: "datacenter 3",
				Rack: []*config.RackConfig{
					{
						Name: "rack 3",
						Switch: []*config.SwitchConfig{
							{
								Name:  "switch 2",
								Ports: 8,
							},
							{
								Name:  "switch 3",
								Ports: 16,
							},
						},
					},
				},
			},
		}
		e := validateDatacenters(context.Background(), datacentersConfig)
		So(e, ShouldBeNil)
	})
}
