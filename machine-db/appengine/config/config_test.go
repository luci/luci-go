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

	"go.chromium.org/luci/common/config/validation"
	"go.chromium.org/luci/machine-db/api/config/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateDatacenters(t *testing.T) {
	t.Parallel()

	Convey("validateDatacenters: empty datacenters config", t, func() {
		context := &validation.Context{}
		datacentersConfig := []*config.DatacenterConfig{}
		validateDatacenters(context, datacentersConfig)
		e := context.Finalize()
		So(e, ShouldBeNil)
	})

	Convey("validateDatacenters: unnamed datacenter", t, func() {
		context := &validation.Context{}
		datacentersConfig := []*config.DatacenterConfig{
			{},
		}
		validateDatacenters(context, datacentersConfig)
		e := context.Finalize()
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "datacenter names are required and must be non-empty")
	})

	Convey("validateDatacenters: duplicate datacenters", t, func() {
		context := &validation.Context{}
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
		validateDatacenters(context, datacentersConfig)
		e := context.Finalize()
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "duplicate datacenter: duplicate")
	})

	Convey("validateDatacenters: unnamed rack", t, func() {
		context := &validation.Context{}
		datacentersConfig := []*config.DatacenterConfig{
			{
				Name: "datacenter",
				Rack: []*config.RackConfig{
					{},
				},
			},
		}
		validateDatacenters(context, datacentersConfig)
		e := context.Finalize()
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "rack names are required and must be non-empty")
	})

	Convey("validateDatacenters: duplicate racks in the same datacenter", t, func() {
		context := &validation.Context{}
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
		validateDatacenters(context, datacentersConfig)
		e := context.Finalize()
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "duplicate rack: duplicate")
	})

	Convey("validateDatacenters: duplicate racks in different datacenters", t, func() {
		context := &validation.Context{}
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
		validateDatacenters(context, datacentersConfig)
		e := context.Finalize()
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "duplicate rack: duplicate")
	})

	Convey("validateDatacenters: unnamed switch", t, func() {
		context := &validation.Context{}
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
		validateDatacenters(context, datacentersConfig)
		e := context.Finalize()
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "switch names are required and must be non-empty")
	})

	Convey("validateDatacenters: duplicate switches in the same rack", t, func() {
		context := &validation.Context{}
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
		validateDatacenters(context, datacentersConfig)
		e := context.Finalize()
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "duplicate switch: duplicate")
	})

	Convey("validateDatacenters: duplicate switches in different racks", t, func() {
		context := &validation.Context{}
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
		validateDatacenters(context, datacentersConfig)
		e := context.Finalize()
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "duplicate switch: duplicate")
	})

	Convey("validateDatacenters: missing switch ports", t, func() {
		context := &validation.Context{}
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
		validateDatacenters(context, datacentersConfig)
		e := context.Finalize()
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "switch must have at least one port: switch")
	})

	Convey("validateDatacenters: negative switch ports", t, func() {
		context := &validation.Context{}
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
		validateDatacenters(context, datacentersConfig)
		e := context.Finalize()
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "switch must have at least one port: switch")
	})

	Convey("validateDatacenters: zero switch ports", t, func() {
		context := &validation.Context{}
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
		validateDatacenters(context, datacentersConfig)
		e := context.Finalize()
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "switch must have at least one port: switch")
	})

	Convey("validateDatacenters: excessive switch ports", t, func() {
		context := &validation.Context{}
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
		validateDatacenters(context, datacentersConfig)
		e := context.Finalize()
		So(e, ShouldNotBeNil)
		So(e, ShouldErrLike, "switch must have at most 65535 ports: switch")
	})

	Convey("validateDatacenters: ok", t, func() {
		context := &validation.Context{}
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
		validateDatacenters(context, datacentersConfig)
		e := context.Finalize()
		So(e, ShouldBeNil)
	})
}
