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

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/gce/api/config/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestVMs(t *testing.T) {
	t.Parallel()

	Convey("VMs", t, func() {
		c := memory.Use(context.Background())
		v := &VMs{ID: "id"}
		err := datastore.Get(c, v)
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		err = datastore.Put(c, &VMs{
			ID: "id",
			Config: config.Block{
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

		err = datastore.Get(c, v)
		So(err, ShouldBeNil)
		So(v, ShouldResemble, &VMs{
			ID: "id",
			Config: config.Block{
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
	})
}

func TestVM(t *testing.T) {
	t.Parallel()

	Convey("VM", t, func() {
		c := memory.Use(context.Background())
		v := &VM{ID: "id"}
		err := datastore.Get(c, v)
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		err = datastore.Put(c, &VM{
			ID: "id",
			Attributes: config.VM{
				Project: "project",
			},
		})
		So(err, ShouldBeNil)

		err = datastore.Get(c, v)
		So(err, ShouldBeNil)
		So(v, ShouldResemble, &VM{
			ID: "id",
			Attributes: config.VM{
				Project: "project",
			},
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

	Convey("GetInstance", t, func() {
		Convey("empty", func() {
			v := &VM{}
			i := v.GetInstance()
			So(i.Disks, ShouldHaveLength, 0)
			So(i.Metadata, ShouldBeNil)
			So(i.NetworkInterfaces, ShouldHaveLength, 0)
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
					MachineType: "type",
					NetworkInterface: []*config.NetworkInterface{
						{
							AccessConfig: []*config.AccessConfig{
								{},
							},
							Network: "network",
						},
					},
				},
			}
			i := v.GetInstance()
			So(i.Disks, ShouldHaveLength, 1)
			So(i.MachineType, ShouldEqual, "type")
			So(i.Metadata, ShouldBeNil)
			So(i.NetworkInterfaces, ShouldHaveLength, 1)
			So(i.ServiceAccounts, ShouldBeNil)
		})
	})
}
