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

package model

import (
	"context"
	"testing"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	computealpha "google.golang.org/api/compute/v0.alpha"
	"google.golang.org/api/compute/v1"
	"google.golang.org/protobuf/testing/protocmp"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/projects/v1"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	Convey("Config", t, func() {
		c := memory.Use(context.Background())
		cfg := &Config{ID: "id"}
		err := datastore.Get(c, cfg)
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		err = datastore.Put(c, &Config{
			ID: "id",
			Config: &config.Config{
				Attributes: &config.VM{
					Disk: []*config.Disk{
						{
							Image: "image",
						},
					},
					Project: "project",
				},
				Prefix: "prefix",
			},
		})
		So(err, ShouldBeNil)

		err = datastore.Get(c, cfg)
		So(err, ShouldBeNil)
		So(cmp.Diff(cfg, &Config{
			ID: "id",
			Config: &config.Config{
				Attributes: &config.VM{
					Disk: []*config.Disk{
						{
							Image: "image",
						},
					},
					Project: "project",
				},
				Prefix: "prefix",
			},
		}, cmpopts.IgnoreUnexported(*cfg), protocmp.Transform()), ShouldBeEmpty)
	})
}

func TestProject(t *testing.T) {
	t.Parallel()

	Convey("Project", t, func() {
		c := memory.Use(context.Background())
		p := &Project{ID: "id"}
		err := datastore.Get(c, p)
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		err = datastore.Put(c, &Project{
			ID: "id",
			Config: &projects.Config{
				Metric: []string{
					"metric-1",
					"metric-2",
				},
				Project: "project",
				Region: []string{
					"region-1",
					"region-2",
				},
			},
		})
		So(err, ShouldBeNil)

		err = datastore.Get(c, p)
		So(err, ShouldBeNil)

		So(p.Config, ShouldResembleProto, &projects.Config{
			Metric: []string{
				"metric-1",
				"metric-2",
			},
			Project: "project",
			Region: []string{
				"region-1",
				"region-2",
			},
		})
	})
}

func TestVM(t *testing.T) {
	t.Parallel()

	Convey("VM", t, func() {
		c := memory.Use(context.Background())
		v := &VM{
			ID: "id",
		}
		err := datastore.Get(c, v)
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		err = datastore.Put(c, &VM{
			ID: "id",
			Attributes: config.VM{
				Project: "project",
			},
		})
		So(err, ShouldBeNil)

		So(datastore.Get(c, v), ShouldBeNil)

		So(cmp.Diff(v, &VM{
			ID: "id",
			Attributes: config.VM{
				Project: "project",
			},
		}, cmpopts.IgnoreUnexported(VM{}), protocmp.Transform()), ShouldBeEmpty)

		Convey("IndexAttributes", func() {
			v.IndexAttributes()
			So(v.AttributesIndexed, ShouldBeEmpty)

			v := &VM{
				ID: "id",
				Attributes: config.VM{
					Disk: []*config.Disk{
						{
							Image: "global/images/image-1",
						},
						{
							Image: "projects/project/global/images/image-2",
						},
					},
				},
			}
			v.IndexAttributes()
			So(v.AttributesIndexed, ShouldResemble, []string{"disk.image:image-1", "disk.image:image-2"})
		})
	})

	Convey("getConfidentialInstanceConfig", t, func() {
		Convey("zero", func() {
			Convey("empty", func() {
				v := &VM{}
				c := v.getConfidentialInstanceConfig()
				So(c, ShouldBeNil)
				s := v.getScheduling()
				So(s, ShouldBeNil)
			})
		})
		Convey("EnableConfidentialCompute", func() {
			v := &VM{
				Attributes: config.VM{
					EnableConfidentialCompute: true,
				},
			}
			c := v.getConfidentialInstanceConfig()
			So(c, ShouldResemble, &compute.ConfidentialInstanceConfig{
				EnableConfidentialCompute: true,
			})
			s := v.getScheduling()
			So(s, ShouldResemble, &compute.Scheduling{
				NodeAffinities:    []*compute.SchedulingNodeAffinity{},
				OnHostMaintenance: "TERMINATE",
			})
		})
	})

	Convey("getDisks", t, func() {
		Convey("zero", func() {
			Convey("nil", func() {
				v := &VM{}
				d := v.getDisks()
				So(d, ShouldHaveLength, 0)
			})

			Convey("empty", func() {
				v := &VM{
					Attributes: config.VM{
						Disk: []*config.Disk{},
					},
				}
				d := v.getDisks()
				So(d, ShouldHaveLength, 0)
			})
		})

		Convey("non-zero", func() {
			Convey("empty", func() {
				v := &VM{
					Attributes: config.VM{
						Disk: []*config.Disk{
							{},
						},
					},
				}
				d := v.getDisks()
				So(d, ShouldHaveLength, 1)
				So(d[0].AutoDelete, ShouldBeTrue)
				So(d[0].Boot, ShouldBeTrue)
				So(d[0].InitializeParams.DiskSizeGb, ShouldEqual, 0)
			})

			Convey("non-empty", func() {
				v := &VM{
					Attributes: config.VM{
						Disk: []*config.Disk{
							{
								Image: "image",
							},
						},
					},
				}
				d := v.getDisks()
				So(d, ShouldHaveLength, 1)
				So(d[0].InitializeParams.SourceImage, ShouldEqual, "image")
			})

			Convey("multi", func() {
				v := &VM{
					Attributes: config.VM{
						Disk: []*config.Disk{
							{},
							{},
						},
					},
				}
				d := v.getDisks()
				So(d, ShouldHaveLength, 2)
				So(d[0].Boot, ShouldBeTrue)
				So(d[1].Boot, ShouldBeFalse)
			})

			Convey("scratch", func() {
				v := &VM{
					Attributes: config.VM{
						Disk: []*config.Disk{
							{
								Type: "zones/zone/diskTypes/pd-ssd",
							},
							{
								Type: "zones/zone/diskTypes/local-ssd",
							},
							{
								Type: "zones/zone/diskTypes/pd-standard",
							},
						},
					},
				}
				d := v.getDisks()
				So(d, ShouldHaveLength, 3)
				So(d[0].Type, ShouldEqual, "")
				So(d[1].Type, ShouldEqual, "SCRATCH")
				So(d[2].Type, ShouldEqual, "")
			})
		})
	})

	Convey("getMetadata", t, func() {
		Convey("nil", func() {
			v := &VM{}
			m := v.getMetadata()
			So(m, ShouldBeNil)
		})

		Convey("empty", func() {
			v := &VM{
				Attributes: config.VM{
					Metadata: []*config.Metadata{},
				},
			}
			m := v.getMetadata()
			So(m, ShouldBeNil)
		})

		Convey("non-empty", func() {
			Convey("empty-nil", func() {
				v := &VM{
					Attributes: config.VM{
						Metadata: []*config.Metadata{
							{},
						},
					},
				}
				m := v.getMetadata()
				So(m.Items, ShouldHaveLength, 1)
				So(m.Items[0].Key, ShouldEqual, "")
				So(m.Items[0].Value, ShouldBeNil)
			})

			Convey("key-nil", func() {
				v := &VM{
					Attributes: config.VM{
						Metadata: []*config.Metadata{
							{
								Metadata: &config.Metadata_FromText{
									FromText: "key",
								},
							},
						},
					},
				}
				m := v.getMetadata()
				So(m.Items, ShouldHaveLength, 1)
				So(m.Items[0].Key, ShouldEqual, "key")
				So(m.Items[0].Value, ShouldBeNil)
			})

			Convey("key-empty", func() {
				v := &VM{
					Attributes: config.VM{
						Metadata: []*config.Metadata{
							{
								Metadata: &config.Metadata_FromText{
									FromText: "key:",
								},
							},
						},
					},
				}
				m := v.getMetadata()
				So(m.Items, ShouldHaveLength, 1)
				So(m.Items[0].Key, ShouldEqual, "key")
				So(*m.Items[0].Value, ShouldEqual, "")
			})

			Convey("key-value", func() {
				v := &VM{
					Attributes: config.VM{
						Metadata: []*config.Metadata{
							{
								Metadata: &config.Metadata_FromText{
									FromText: "key:value",
								},
							},
						},
					},
				}
				m := v.getMetadata()
				So(m.Items, ShouldHaveLength, 1)
				So(m.Items[0].Key, ShouldEqual, "key")
				So(*m.Items[0].Value, ShouldEqual, "value")
			})

			Convey("empty-value", func() {
				v := &VM{
					Attributes: config.VM{
						Metadata: []*config.Metadata{
							{
								Metadata: &config.Metadata_FromText{
									FromText: ":value",
								},
							},
						},
					},
				}
				m := v.getMetadata()
				So(m.Items, ShouldHaveLength, 1)
				So(m.Items[0].Key, ShouldEqual, "")
				So(*m.Items[0].Value, ShouldEqual, "value")
			})

			Convey("from file", func() {
				v := &VM{
					Attributes: config.VM{
						Metadata: []*config.Metadata{
							{
								Metadata: &config.Metadata_FromFile{
									FromFile: "key:file",
								},
							},
						},
					},
				}
				m := v.getMetadata()
				So(m.Items, ShouldHaveLength, 1)
				So(m.Items[0].Key, ShouldEqual, "")
				So(m.Items[0].Value, ShouldBeNil)
			})
		})
	})

	Convey("getNetworkInterfaces", t, func() {
		Convey("zero", func() {
			Convey("nil", func() {
				v := &VM{}
				n := v.getNetworkInterfaces()
				So(n, ShouldHaveLength, 0)
			})

			Convey("empty", func() {
				v := &VM{
					Attributes: config.VM{
						NetworkInterface: []*config.NetworkInterface{},
					},
				}
				n := v.getNetworkInterfaces()
				So(n, ShouldHaveLength, 0)
			})
		})

		Convey("non-zero", func() {
			Convey("empty", func() {
				v := &VM{
					Attributes: config.VM{
						NetworkInterface: []*config.NetworkInterface{
							{},
						},
					},
				}
				n := v.getNetworkInterfaces()
				So(n, ShouldHaveLength, 1)
				So(n[0].AccessConfigs, ShouldHaveLength, 0)
				So(n[0].Network, ShouldEqual, "")
			})

			Convey("non-empty", func() {
				Convey("network and subnetwork", func() {
					v := &VM{
						Attributes: config.VM{
							NetworkInterface: []*config.NetworkInterface{
								{
									AccessConfig: []*config.AccessConfig{},
									Network:      "network",
									Subnetwork:   "subnetwork",
								},
							},
						},
					}
					n := v.getNetworkInterfaces()
					So(n, ShouldHaveLength, 1)
					So(n[0].AccessConfigs, ShouldBeNil)
					So(n[0].Network, ShouldEqual, "network")
					So(n[0].Subnetwork, ShouldEqual, "subnetwork")
				})

				Convey("network", func() {
					v := &VM{
						Attributes: config.VM{
							NetworkInterface: []*config.NetworkInterface{
								{
									AccessConfig: []*config.AccessConfig{},
									Network:      "network",
								},
							},
						},
					}
					n := v.getNetworkInterfaces()
					So(n, ShouldHaveLength, 1)
					So(n[0].AccessConfigs, ShouldBeNil)
					So(n[0].Network, ShouldEqual, "network")
				})

				Convey("subnetwork", func() {
					v := &VM{
						Attributes: config.VM{
							NetworkInterface: []*config.NetworkInterface{
								{
									AccessConfig: []*config.AccessConfig{},
									Subnetwork:   "subnetwork",
								},
							},
						},
					}
					n := v.getNetworkInterfaces()
					So(n, ShouldHaveLength, 1)
					So(n[0].AccessConfigs, ShouldBeNil)
					So(n[0].Subnetwork, ShouldEqual, "subnetwork")
				})

				Convey("access configs", func() {
					v := &VM{
						Attributes: config.VM{
							NetworkInterface: []*config.NetworkInterface{
								{
									AccessConfig: []*config.AccessConfig{
										{
											Type: config.AccessConfigType_ONE_TO_ONE_NAT,
										},
									},
								},
							},
						},
					}
					n := v.getNetworkInterfaces()
					So(n, ShouldHaveLength, 1)
					So(n[0].AccessConfigs, ShouldHaveLength, 1)
					So(n[0].AccessConfigs[0].Type, ShouldEqual, "ONE_TO_ONE_NAT")
				})
			})
		})
	})

	Convey("getServiceAccounts", t, func() {
		Convey("zero", func() {
			Convey("nil", func() {
				v := &VM{}
				s := v.getServiceAccounts()
				So(s, ShouldHaveLength, 0)
			})

			Convey("empty", func() {
				v := &VM{
					Attributes: config.VM{
						ServiceAccount: []*config.ServiceAccount{},
					},
				}
				s := v.getServiceAccounts()
				So(s, ShouldHaveLength, 0)
			})
		})

		Convey("non-zero", func() {
			Convey("empty", func() {
				v := &VM{
					Attributes: config.VM{
						ServiceAccount: []*config.ServiceAccount{
							{},
						},
					},
				}
				s := v.getServiceAccounts()
				So(s, ShouldHaveLength, 1)
				So(s[0].Email, ShouldEqual, "")
				So(s[0].Scopes, ShouldHaveLength, 0)
			})

			Convey("non-empty", func() {
				Convey("email", func() {
					v := &VM{
						Attributes: config.VM{
							ServiceAccount: []*config.ServiceAccount{
								{
									Email: "email",
									Scope: []string{},
								},
							},
						},
					}
					s := v.getServiceAccounts()
					So(s, ShouldHaveLength, 1)
					So(s[0].Email, ShouldEqual, "email")
					So(s[0].Scopes, ShouldHaveLength, 0)
				})

				Convey("scopes", func() {
					v := &VM{
						Attributes: config.VM{
							ServiceAccount: []*config.ServiceAccount{
								{
									Scope: []string{
										"scope",
									},
								},
							},
						},
					}
					s := v.getServiceAccounts()
					So(s, ShouldHaveLength, 1)
					So(s[0].Email, ShouldEqual, "")
					So(s[0].Scopes, ShouldHaveLength, 1)
					So(s[0].Scopes[0], ShouldEqual, "scope")
				})
			})
		})

	})

	Convey("getScheduling", t, func() {
		Convey("zero", func() {
			Convey("nil", func() {
				v := &VM{}
				s := v.getScheduling()
				So(s, ShouldBeNil)
			})

			Convey("empty", func() {
				v := &VM{
					Attributes: config.VM{
						Scheduling: &config.Scheduling{
							NodeAffinity: []*config.NodeAffinity{},
						},
					},
				}
				s := v.getScheduling()
				So(s, ShouldBeNil)
			})
		})

		Convey("non-zero", func() {
			Convey("empty", func() {
				v := &VM{
					Attributes: config.VM{
						Scheduling: &config.Scheduling{
							NodeAffinity: []*config.NodeAffinity{
								{},
							},
						},
					},
				}
				s := v.getScheduling()
				So(s, ShouldNotBeNil)
				So(s.NodeAffinities, ShouldHaveLength, 1)
				So(s.NodeAffinities[0].Key, ShouldEqual, "")
				So(s.NodeAffinities[0].Operator, ShouldEqual, "OPERATOR_UNSPECIFIED")
				So(s.NodeAffinities[0].Values, ShouldHaveLength, 0)
			})

			Convey("non-empty", func() {
				Convey("key", func() {
					v := &VM{
						Attributes: config.VM{
							Scheduling: &config.Scheduling{
								NodeAffinity: []*config.NodeAffinity{
									{
										Key: "node-affinity-key",
									},
								},
							},
						},
					}
					s := v.getScheduling()
					So(s.NodeAffinities, ShouldHaveLength, 1)
					So(s.NodeAffinities[0].Key, ShouldEqual, "node-affinity-key")
				})
				Convey("operator", func() {
					v := &VM{
						Attributes: config.VM{
							Scheduling: &config.Scheduling{
								NodeAffinity: []*config.NodeAffinity{
									{
										Operator: config.NodeAffinityOperator_IN,
									},
								},
							},
						},
					}
					s := v.getScheduling()
					So(s.NodeAffinities, ShouldHaveLength, 1)
					So(s.NodeAffinities[0].Operator, ShouldEqual, "IN")
				})
				Convey("values", func() {
					v := &VM{
						Attributes: config.VM{
							Scheduling: &config.Scheduling{
								NodeAffinity: []*config.NodeAffinity{
									{
										Values: []string{"node-affinity-value"},
									},
								},
							},
						},
					}
					s := v.getScheduling()
					So(s.NodeAffinities, ShouldHaveLength, 1)
					So(s.NodeAffinities[0].Values, ShouldHaveLength, 1)
					So(s.NodeAffinities[0].Values[0], ShouldEqual, "node-affinity-value")
				})
			})
		})
	})

	Convey("getShieldedInstanceConfig", t, func() {
		Convey("zero", func() {
			Convey("empty", func() {
				v := &VM{}
				c := v.getShieldedInstanceConfig()
				So(c, ShouldBeNil)
			})
		})
		Convey("non-zero", func() {
			Convey("disableIntegrityMonitoring", func() {
				v := &VM{
					Attributes: config.VM{
						DisableIntegrityMonitoring: true,
					},
				}
				c := v.getShieldedInstanceConfig()
				So(c, ShouldResemble, &compute.ShieldedInstanceConfig{
					EnableIntegrityMonitoring: false,
					EnableSecureBoot:          false,
					EnableVtpm:                true,
				})
			})
			Convey("enableSecureBoot", func() {
				v := &VM{
					Attributes: config.VM{
						EnableSecureBoot: true,
					},
				}
				c := v.getShieldedInstanceConfig()
				So(c, ShouldResemble, &compute.ShieldedInstanceConfig{
					EnableIntegrityMonitoring: true,
					EnableSecureBoot:          true,
					EnableVtpm:                true,
				})
			})
			Convey("disablevTPM", func() {
				v := &VM{
					Attributes: config.VM{
						DisableVtpm: true,
					},
				}
				c := v.getShieldedInstanceConfig()
				So(c, ShouldResemble, &compute.ShieldedInstanceConfig{
					EnableIntegrityMonitoring: true,
					EnableSecureBoot:          false,
					EnableVtpm:                false,
				})
			})
		})
	})

	Convey("getTags", t, func() {
		Convey("zero", func() {
			Convey("nil", func() {
				v := &VM{}
				t := v.getTags()
				So(t, ShouldBeNil)
			})

			Convey("empty", func() {
				v := &VM{
					Attributes: config.VM{
						Tag: []string{},
					},
				}
				t := v.getTags()
				So(t, ShouldBeNil)
			})
		})

		Convey("non-zero", func() {
			v := &VM{
				Attributes: config.VM{
					Tag: []string{
						"tag",
					},
				},
			}
			t := v.getTags()
			So(t.Items, ShouldHaveLength, 1)
			So(t.Items[0], ShouldEqual, "tag")
		})
	})

	Convey("GetLabels", t, func() {
		Convey("zero", func() {
			Convey("nil", func() {
				v := &VM{}
				l := v.getLabels()
				So(l, ShouldBeEmpty)
			})

			Convey("empty", func() {
				v := &VM{
					Attributes: config.VM{
						Label: map[string]string{},
					},
				}
				l := v.getLabels()
				So(l, ShouldBeEmpty)
			})
		})

		Convey("non-zero", func() {
			v := &VM{
				Attributes: config.VM{
					Label: map[string]string{"key1": "value1"},
				},
			}
			l := v.getLabels()
			So(l, ShouldHaveLength, 1)
			So(l, ShouldContainKey, "key1")
			So(l["key1"], ShouldEqual, "value1")
		})
	})

	Convey("GetInstance", t, func() {
		Convey("empty", func() {
			v := &VM{}
			i := v.GetInstance().Stable
			So(i.Disks, ShouldHaveLength, 0)
			So(i.MachineType, ShouldEqual, "")
			So(i.Metadata, ShouldBeNil)
			So(i.MinCpuPlatform, ShouldEqual, "")
			So(i.NetworkInterfaces, ShouldHaveLength, 0)
			So(i.Scheduling, ShouldBeNil)
			So(i.ServiceAccounts, ShouldBeNil)
			So(i.ShieldedInstanceConfig, ShouldBeNil)
			So(i.Tags, ShouldBeNil)
			So(i.Labels, ShouldBeNil)
			So(i.ForceSendFields, ShouldBeNil)
		})

		Convey("non-empty", func() {
			v := &VM{
				Attributes: config.VM{
					Disk: []*config.Disk{
						{
							Image: "image",
							Size:  100,
						},
					},
					EnableSecureBoot: true,
					MachineType:      "type",
					MinCpuPlatform:   "plat",
					NetworkInterface: []*config.NetworkInterface{
						{
							AccessConfig: []*config.AccessConfig{
								{},
							},
							Network: "network",
						},
					},
					Scheduling: &config.Scheduling{
						NodeAffinity: []*config.NodeAffinity{
							{},
						},
					},
					ForceSendFields: []string{
						"a",
						"b",
						"c",
					},
					NullFields: []string{
						"e",
					},
				},
			}
			i := v.GetInstance().Stable
			So(i.Disks, ShouldHaveLength, 1)
			So(i.MachineType, ShouldEqual, "type")
			So(i.Metadata, ShouldBeNil)
			So(i.MinCpuPlatform, ShouldEqual, "plat")
			So(i.NetworkInterfaces, ShouldHaveLength, 1)
			So(i.ServiceAccounts, ShouldBeNil)
			So(i.Scheduling, ShouldNotBeNil)
			So(i.Scheduling.NodeAffinities, ShouldHaveLength, 1)
			So(i.ShieldedInstanceConfig, ShouldNotBeNil)
			So(i.Tags, ShouldBeNil)
			So(i.Labels, ShouldBeNil)
			So(i.ForceSendFields, ShouldNotBeNil)
			So(i.NullFields, ShouldNotBeNil)
		})
	})
}

// TestGetInstanceReturnsAlpha tests the conversion of various input VMs to the
// alpha GCP API.
func TestGetInstanceReturnsAlpha(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		input  *VM
		output *computealpha.Instance
	}{
		{
			name: "only alpha",
			input: &VM{
				Attributes: config.VM{
					GcpChannel: config.GCPChannel_GCP_CHANNEL_ALPHA,
				},
			},
			output: &computealpha.Instance{},
		},
		{
			name: "hostname",
			input: &VM{
				Hostname: "awesome-host",
				Attributes: config.VM{
					GcpChannel: config.GCPChannel_GCP_CHANNEL_ALPHA,
				},
			},
			output: &computealpha.Instance{
				Name: "awesome-host",
			},
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			expected := tt.output
			actual := tt.input.GetInstance().Alpha

			if diff := cmp.Diff(expected, actual, protocmp.Transform()); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

// LegacyConfigV1 is the type of an old Config struct.
// This type is used to ensure that the current (as of 2023-09-25) struct and
// the former version are compatible within datastore, i.e. records can be written
// or read with the old or new schema.
type LegacyConfigV1 struct {
	_extra datastore.PropertyMap `gae:"-,extra"`
	_kind  string                `gae:"$kind,Config"`
	ID     string                `gae:"$id"`
	Config config.Config         `gae:"binary_config,noindex"`
}

// TestReadLegacyConfigV1AsConfig tests writing a record to in-memory datastore as a
// LegacyConfigV1 and reading it out as a Config.
//
// This test is more valuable than just being a leaf test.
// Basically, the old Config field, which is a config.Config not a *config.Config,
// copies mutex fields inside a proto. We are getting rid of this by changing the type
// of the field to be a pointer and changing some of the annotations.
//
// This kind of schema change should be transparent and harmless, but I don't know that for sure.
//
// This test's purpose is to catch problems with updating datastore schemas in go projects, so
// if we encounter a problem in prod, we will go back and update this test in such a way that it
// would have caught the issue.
func TestReadLegacyConfigV1AsConfig(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	input := &LegacyConfigV1{
		ID: "a",
		Config: config.Config{
			Swarming: "ed18ba13-bfe7-46bc-ae77-77ec70f2c22c",
		},
	}
	output := &Config{
		ID: "a",
		Config: &config.Config{
			Swarming: "ed18ba13-bfe7-46bc-ae77-77ec70f2c22c",
		},
	}

	if err := datastore.Put(ctx, input); err != nil {
		t.Errorf("error when inserting record to datastore: %s", err)
	}

	expected := output
	actual := &Config{
		ID: "a",
	}
	if err := datastore.Get(ctx, actual); err != nil {
		t.Errorf("error when retrieving record from datastore: %s", err)
	}

	if diff := cmp.Diff(expected, actual, protocmp.Transform(), cmpopts.IgnoreUnexported(Config{})); diff != "" {
		t.Errorf("unexpected diff (-want +got): %s", diff)
	}
}
