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

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	computealpha "google.golang.org/api/compute/v0.alpha"
	"google.golang.org/api/compute/v1"
	"google.golang.org/protobuf/testing/protocmp"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/projects/v1"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("Config", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		cfg := &Config{ID: "id"}
		err := datastore.Get(c, cfg)
		assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))

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
		assert.Loosely(t, err, should.BeNil)

		err = datastore.Get(c, cfg)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cmp.Diff(cfg, &Config{
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
		}, cmpopts.IgnoreUnexported(*cfg), protocmp.Transform()), should.BeEmpty)
	})
}

func TestProject(t *testing.T) {
	t.Parallel()

	ftt.Run("Project", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		p := &Project{ID: "id"}
		err := datastore.Get(c, p)
		assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))

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
		assert.Loosely(t, err, should.BeNil)

		err = datastore.Get(c, p)
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, p.Config, should.Resemble(&projects.Config{
			Metric: []string{
				"metric-1",
				"metric-2",
			},
			Project: "project",
			Region: []string{
				"region-1",
				"region-2",
			},
		}))
	})
}

func TestVM(t *testing.T) {
	t.Parallel()

	ftt.Run("VM", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		v := &VM{
			ID: "id",
		}
		err := datastore.Get(c, v)
		assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))

		err = datastore.Put(c, &VM{
			ID: "id",
			Attributes: config.VM{
				Project: "project",
			},
		})
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, datastore.Get(c, v), should.BeNil)

		assert.Loosely(t, cmp.Diff(v, &VM{
			ID: "id",
			Attributes: config.VM{
				Project: "project",
			},
		}, cmpopts.IgnoreUnexported(VM{}), protocmp.Transform()), should.BeEmpty)

		t.Run("IndexAttributes", func(t *ftt.Test) {
			v.IndexAttributes()
			assert.Loosely(t, v.AttributesIndexed, should.BeEmpty)

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
			assert.Loosely(t, v.AttributesIndexed, should.Resemble([]string{"disk.image:image-1", "disk.image:image-2"}))
		})
	})

	ftt.Run("getConfidentialInstanceConfig", t, func(t *ftt.Test) {
		t.Run("zero", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				v := &VM{}
				c := v.getConfidentialInstanceConfig()
				assert.Loosely(t, c, should.BeNil)
				s := v.getScheduling()
				assert.Loosely(t, s, should.BeNil)
			})
		})
		t.Run("EnableConfidentialCompute", func(t *ftt.Test) {
			v := &VM{
				Attributes: config.VM{
					EnableConfidentialCompute: true,
				},
			}
			c := v.getConfidentialInstanceConfig()
			assert.Loosely(t, c, should.Resemble(&compute.ConfidentialInstanceConfig{
				EnableConfidentialCompute: true,
			}))
			s := v.getScheduling()
			assert.Loosely(t, s, should.Resemble(&compute.Scheduling{
				NodeAffinities:    []*compute.SchedulingNodeAffinity{},
				OnHostMaintenance: "TERMINATE",
			}))
		})
	})

	ftt.Run("getDisks", t, func(t *ftt.Test) {
		t.Run("zero", func(t *ftt.Test) {
			t.Run("nil", func(t *ftt.Test) {
				v := &VM{}
				d := v.getDisks()
				assert.Loosely(t, d, should.HaveLength(0))
			})

			t.Run("empty", func(t *ftt.Test) {
				v := &VM{
					Attributes: config.VM{
						Disk: []*config.Disk{},
					},
				}
				d := v.getDisks()
				assert.Loosely(t, d, should.HaveLength(0))
			})
		})

		t.Run("non-zero", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				v := &VM{
					Attributes: config.VM{
						Disk: []*config.Disk{
							{},
						},
					},
				}
				d := v.getDisks()
				assert.Loosely(t, d, should.HaveLength(1))
				assert.Loosely(t, d[0].AutoDelete, should.BeTrue)
				assert.Loosely(t, d[0].Boot, should.BeTrue)
				assert.Loosely(t, d[0].InitializeParams.DiskSizeGb, should.BeZero)
			})

			t.Run("non-empty", func(t *ftt.Test) {
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
				assert.Loosely(t, d, should.HaveLength(1))
				assert.Loosely(t, d[0].InitializeParams.SourceImage, should.Equal("image"))
			})

			t.Run("multi", func(t *ftt.Test) {
				v := &VM{
					Attributes: config.VM{
						Disk: []*config.Disk{
							{},
							{},
						},
					},
				}
				d := v.getDisks()
				assert.Loosely(t, d, should.HaveLength(2))
				assert.Loosely(t, d[0].Boot, should.BeTrue)
				assert.Loosely(t, d[1].Boot, should.BeFalse)
			})

			t.Run("scratch", func(t *ftt.Test) {
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
				assert.Loosely(t, d, should.HaveLength(3))
				assert.Loosely(t, d[0].Type, should.BeEmpty)
				assert.Loosely(t, d[1].Type, should.Equal("SCRATCH"))
				assert.Loosely(t, d[2].Type, should.BeEmpty)
			})
		})
	})

	ftt.Run("getMetadata", t, func(t *ftt.Test) {
		t.Run("nil", func(t *ftt.Test) {
			v := &VM{}
			m := v.getMetadata()
			assert.Loosely(t, m, should.BeNil)
		})

		t.Run("empty", func(t *ftt.Test) {
			v := &VM{
				Attributes: config.VM{
					Metadata: []*config.Metadata{},
				},
			}
			m := v.getMetadata()
			assert.Loosely(t, m, should.BeNil)
		})

		t.Run("non-empty", func(t *ftt.Test) {
			t.Run("empty-nil", func(t *ftt.Test) {
				v := &VM{
					Attributes: config.VM{
						Metadata: []*config.Metadata{
							{},
						},
					},
				}
				m := v.getMetadata()
				assert.Loosely(t, m.Items, should.HaveLength(1))
				assert.Loosely(t, m.Items[0].Key, should.BeEmpty)
				assert.Loosely(t, m.Items[0].Value, should.BeNil)
			})

			t.Run("key-nil", func(t *ftt.Test) {
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
				assert.Loosely(t, m.Items, should.HaveLength(1))
				assert.Loosely(t, m.Items[0].Key, should.Equal("key"))
				assert.Loosely(t, m.Items[0].Value, should.BeNil)
			})

			t.Run("key-empty", func(t *ftt.Test) {
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
				assert.Loosely(t, m.Items, should.HaveLength(1))
				assert.Loosely(t, m.Items[0].Key, should.Equal("key"))
				assert.Loosely(t, *m.Items[0].Value, should.BeEmpty)
			})

			t.Run("key-value", func(t *ftt.Test) {
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
				assert.Loosely(t, m.Items, should.HaveLength(1))
				assert.Loosely(t, m.Items[0].Key, should.Equal("key"))
				assert.Loosely(t, *m.Items[0].Value, should.Equal("value"))
			})

			t.Run("empty-value", func(t *ftt.Test) {
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
				assert.Loosely(t, m.Items, should.HaveLength(1))
				assert.Loosely(t, m.Items[0].Key, should.BeEmpty)
				assert.Loosely(t, *m.Items[0].Value, should.Equal("value"))
			})

			t.Run("from file", func(t *ftt.Test) {
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
				assert.Loosely(t, m.Items, should.HaveLength(1))
				assert.Loosely(t, m.Items[0].Key, should.BeEmpty)
				assert.Loosely(t, m.Items[0].Value, should.BeNil)
			})

			t.Run("empty with dut", func(t *ftt.Test) {
				v := &VM{
					Attributes: config.VM{
						Metadata: []*config.Metadata{},
					},
					DUT: "dut-1",
				}
				m := v.getMetadata()
				assert.Loosely(t, m.Items, should.HaveLength(1))
				assert.Loosely(t, m.Items[0].Key, should.Equal("dut"))
				assert.Loosely(t, *m.Items[0].Value, should.Equal("dut-1"))
			})

			t.Run("non-empty with dut", func(t *ftt.Test) {
				v := &VM{
					Attributes: config.VM{
						Metadata: []*config.Metadata{
							{
								Metadata: &config.Metadata_FromText{
									FromText: "key:file",
								},
							},
						},
					},
					DUT: "dut-1",
				}
				m := v.getMetadata()
				assert.Loosely(t, m.Items, should.HaveLength(2))
				assert.Loosely(t, m.Items[0].Key, should.Equal("key"))
				assert.Loosely(t, *m.Items[0].Value, should.Equal("file"))
				assert.Loosely(t, m.Items[1].Key, should.Equal("dut"))
				assert.Loosely(t, *m.Items[1].Value, should.Equal("dut-1"))
			})
		})
	})

	ftt.Run("getNetworkInterfaces", t, func(t *ftt.Test) {
		t.Run("zero", func(t *ftt.Test) {
			t.Run("nil", func(t *ftt.Test) {
				v := &VM{}
				n := v.getNetworkInterfaces()
				assert.Loosely(t, n, should.HaveLength(0))
			})

			t.Run("empty", func(t *ftt.Test) {
				v := &VM{
					Attributes: config.VM{
						NetworkInterface: []*config.NetworkInterface{},
					},
				}
				n := v.getNetworkInterfaces()
				assert.Loosely(t, n, should.HaveLength(0))
			})
		})

		t.Run("non-zero", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				v := &VM{
					Attributes: config.VM{
						NetworkInterface: []*config.NetworkInterface{
							{},
						},
					},
				}
				n := v.getNetworkInterfaces()
				assert.Loosely(t, n, should.HaveLength(1))
				assert.Loosely(t, n[0].AccessConfigs, should.HaveLength(0))
				assert.Loosely(t, n[0].Network, should.BeEmpty)
			})

			t.Run("non-empty", func(t *ftt.Test) {
				t.Run("network and subnetwork", func(t *ftt.Test) {
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
					assert.Loosely(t, n, should.HaveLength(1))
					assert.Loosely(t, n[0].AccessConfigs, should.BeNil)
					assert.Loosely(t, n[0].Network, should.Equal("network"))
					assert.Loosely(t, n[0].Subnetwork, should.Equal("subnetwork"))
				})

				t.Run("network", func(t *ftt.Test) {
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
					assert.Loosely(t, n, should.HaveLength(1))
					assert.Loosely(t, n[0].AccessConfigs, should.BeNil)
					assert.Loosely(t, n[0].Network, should.Equal("network"))
				})

				t.Run("subnetwork", func(t *ftt.Test) {
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
					assert.Loosely(t, n, should.HaveLength(1))
					assert.Loosely(t, n[0].AccessConfigs, should.BeNil)
					assert.Loosely(t, n[0].Subnetwork, should.Equal("subnetwork"))
				})

				t.Run("access configs", func(t *ftt.Test) {
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
					assert.Loosely(t, n, should.HaveLength(1))
					assert.Loosely(t, n[0].AccessConfigs, should.HaveLength(1))
					assert.Loosely(t, n[0].AccessConfigs[0].Type, should.Equal("ONE_TO_ONE_NAT"))
				})
			})
		})
	})

	ftt.Run("getServiceAccounts", t, func(t *ftt.Test) {
		t.Run("zero", func(t *ftt.Test) {
			t.Run("nil", func(t *ftt.Test) {
				v := &VM{}
				s := v.getServiceAccounts()
				assert.Loosely(t, s, should.HaveLength(0))
			})

			t.Run("empty", func(t *ftt.Test) {
				v := &VM{
					Attributes: config.VM{
						ServiceAccount: []*config.ServiceAccount{},
					},
				}
				s := v.getServiceAccounts()
				assert.Loosely(t, s, should.HaveLength(0))
			})
		})

		t.Run("non-zero", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				v := &VM{
					Attributes: config.VM{
						ServiceAccount: []*config.ServiceAccount{
							{},
						},
					},
				}
				s := v.getServiceAccounts()
				assert.Loosely(t, s, should.HaveLength(1))
				assert.Loosely(t, s[0].Email, should.BeEmpty)
				assert.Loosely(t, s[0].Scopes, should.HaveLength(0))
			})

			t.Run("non-empty", func(t *ftt.Test) {
				t.Run("email", func(t *ftt.Test) {
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
					assert.Loosely(t, s, should.HaveLength(1))
					assert.Loosely(t, s[0].Email, should.Equal("email"))
					assert.Loosely(t, s[0].Scopes, should.HaveLength(0))
				})

				t.Run("scopes", func(t *ftt.Test) {
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
					assert.Loosely(t, s, should.HaveLength(1))
					assert.Loosely(t, s[0].Email, should.BeEmpty)
					assert.Loosely(t, s[0].Scopes, should.HaveLength(1))
					assert.Loosely(t, s[0].Scopes[0], should.Equal("scope"))
				})
			})
		})

	})

	ftt.Run("getScheduling", t, func(t *ftt.Test) {
		t.Run("zero", func(t *ftt.Test) {
			t.Run("nil", func(t *ftt.Test) {
				v := &VM{}
				s := v.getScheduling()
				assert.Loosely(t, s, should.BeNil)
			})

			t.Run("empty", func(t *ftt.Test) {
				v := &VM{
					Attributes: config.VM{
						Scheduling: &config.Scheduling{
							NodeAffinity: []*config.NodeAffinity{},
						},
					},
				}
				s := v.getScheduling()
				assert.Loosely(t, s, should.BeNil)
			})
			t.Run("empty & Confidential", func(t *ftt.Test) {
				v := &VM{
					Attributes: config.VM{
						EnableConfidentialCompute: true,
					},
				}
				s := v.getScheduling()
				assert.Loosely(t, s, should.Resemble(&compute.Scheduling{
					NodeAffinities:    []*compute.SchedulingNodeAffinity{},
					OnHostMaintenance: "TERMINATE",
				}))
			})
			t.Run("empty & Terminatable", func(t *ftt.Test) {
				v := &VM{
					Attributes: config.VM{
						TerminateOnMaintenance: true,
					},
				}
				s := v.getScheduling()
				assert.Loosely(t, s, should.Resemble(&compute.Scheduling{
					NodeAffinities:    []*compute.SchedulingNodeAffinity{},
					OnHostMaintenance: "TERMINATE",
				}))
			})
			t.Run("empty & Confidential & Terminatable", func(t *ftt.Test) {
				v := &VM{
					Attributes: config.VM{
						EnableConfidentialCompute: true,
						TerminateOnMaintenance:    true,
					},
				}
				s := v.getScheduling()
				assert.Loosely(t, s, should.Resemble(&compute.Scheduling{
					NodeAffinities:    []*compute.SchedulingNodeAffinity{},
					OnHostMaintenance: "TERMINATE",
				}))
			})
		})

		t.Run("non-zero", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
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
				assert.Loosely(t, s, should.NotBeNil)
				assert.Loosely(t, s.NodeAffinities, should.HaveLength(1))
				assert.Loosely(t, s.NodeAffinities[0].Key, should.BeEmpty)
				assert.Loosely(t, s.NodeAffinities[0].Operator, should.Equal("OPERATOR_UNSPECIFIED"))
				assert.Loosely(t, s.NodeAffinities[0].Values, should.HaveLength(0))
			})

			t.Run("non-empty", func(t *ftt.Test) {
				t.Run("key", func(t *ftt.Test) {
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
					assert.Loosely(t, s.NodeAffinities, should.HaveLength(1))
					assert.Loosely(t, s.NodeAffinities[0].Key, should.Equal("node-affinity-key"))
				})
				t.Run("operator", func(t *ftt.Test) {
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
					assert.Loosely(t, s.NodeAffinities, should.HaveLength(1))
					assert.Loosely(t, s.NodeAffinities[0].Operator, should.Equal("IN"))
				})
				t.Run("values", func(t *ftt.Test) {
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
					assert.Loosely(t, s.NodeAffinities, should.HaveLength(1))
					assert.Loosely(t, s.NodeAffinities[0].Values, should.HaveLength(1))
					assert.Loosely(t, s.NodeAffinities[0].Values[0], should.Equal("node-affinity-value"))
				})
				t.Run("not-empty other cases", func(t *ftt.Test) {
					inScheduling := &config.Scheduling{
						NodeAffinity: []*config.NodeAffinity{{Key: "node-affinity-key"}},
					}
					t.Run("Confidential", func(t *ftt.Test) {
						v := &VM{
							Attributes: config.VM{
								Scheduling:                inScheduling,
								EnableConfidentialCompute: true,
							},
						}
						s := v.getScheduling()
						assert.Loosely(t, s.NodeAffinities, should.HaveLength(1))
						assert.Loosely(t, s.NodeAffinities[0].Key, should.Equal("node-affinity-key"))
						assert.Loosely(t, s.OnHostMaintenance, should.Equal("TERMINATE"))
					})
					t.Run("Terminatable", func(t *ftt.Test) {
						v := &VM{
							Attributes: config.VM{
								Scheduling:             inScheduling,
								TerminateOnMaintenance: true,
							},
						}
						s := v.getScheduling()
						assert.Loosely(t, s.NodeAffinities, should.HaveLength(1))
						assert.Loosely(t, s.NodeAffinities[0].Key, should.Equal("node-affinity-key"))
						assert.Loosely(t, s.OnHostMaintenance, should.Equal("TERMINATE"))
					})
					t.Run("Confidential & Terminatable", func(t *ftt.Test) {
						v := &VM{
							Attributes: config.VM{
								Scheduling:                inScheduling,
								EnableConfidentialCompute: true,
								TerminateOnMaintenance:    true,
							},
						}
						s := v.getScheduling()
						assert.Loosely(t, s.NodeAffinities, should.HaveLength(1))
						assert.Loosely(t, s.NodeAffinities[0].Key, should.Equal("node-affinity-key"))
						assert.Loosely(t, s.OnHostMaintenance, should.Equal("TERMINATE"))
					})
				})
			})
		})
	})

	ftt.Run("getShieldedInstanceConfig", t, func(t *ftt.Test) {
		t.Run("zero", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				v := &VM{}
				c := v.getShieldedInstanceConfig()
				assert.Loosely(t, c, should.BeNil)
			})
		})
		t.Run("non-zero", func(t *ftt.Test) {
			t.Run("disableIntegrityMonitoring", func(t *ftt.Test) {
				v := &VM{
					Attributes: config.VM{
						DisableIntegrityMonitoring: true,
					},
				}
				c := v.getShieldedInstanceConfig()
				assert.Loosely(t, c, should.Resemble(&compute.ShieldedInstanceConfig{
					EnableIntegrityMonitoring: false,
					EnableSecureBoot:          false,
					EnableVtpm:                true,
				}))
			})
			t.Run("enableSecureBoot", func(t *ftt.Test) {
				v := &VM{
					Attributes: config.VM{
						EnableSecureBoot: true,
					},
				}
				c := v.getShieldedInstanceConfig()
				assert.Loosely(t, c, should.Resemble(&compute.ShieldedInstanceConfig{
					EnableIntegrityMonitoring: true,
					EnableSecureBoot:          true,
					EnableVtpm:                true,
				}))
			})
			t.Run("disablevTPM", func(t *ftt.Test) {
				v := &VM{
					Attributes: config.VM{
						DisableVtpm: true,
					},
				}
				c := v.getShieldedInstanceConfig()
				assert.Loosely(t, c, should.Resemble(&compute.ShieldedInstanceConfig{
					EnableIntegrityMonitoring: true,
					EnableSecureBoot:          false,
					EnableVtpm:                false,
				}))
			})
		})
	})

	ftt.Run("getTags", t, func(t *ftt.Test) {
		t.Run("zero", func(t *ftt.Test) {
			t.Run("nil", func(t *ftt.Test) {
				v := &VM{}
				tags := v.getTags()
				assert.Loosely(t, tags, should.BeNil)
			})

			t.Run("empty", func(t *ftt.Test) {
				v := &VM{
					Attributes: config.VM{
						Tag: []string{},
					},
				}
				tags := v.getTags()
				assert.Loosely(t, tags, should.BeNil)
			})
		})

		t.Run("non-zero", func(t *ftt.Test) {
			v := &VM{
				Attributes: config.VM{
					Tag: []string{
						"tag",
					},
				},
			}
			tags := v.getTags()
			assert.Loosely(t, tags.Items, should.HaveLength(1))
			assert.Loosely(t, tags.Items[0], should.Equal("tag"))
		})
	})

	ftt.Run("GetLabels", t, func(t *ftt.Test) {
		t.Run("zero", func(t *ftt.Test) {
			t.Run("nil", func(t *ftt.Test) {
				v := &VM{}
				l := v.getLabels()
				assert.Loosely(t, l, should.BeEmpty)
			})

			t.Run("empty", func(t *ftt.Test) {
				v := &VM{
					Attributes: config.VM{
						Label: map[string]string{},
					},
				}
				l := v.getLabels()
				assert.Loosely(t, l, should.BeEmpty)
			})
		})

		t.Run("non-zero", func(t *ftt.Test) {
			v := &VM{
				Attributes: config.VM{
					Label: map[string]string{"key1": "value1"},
				},
			}
			l := v.getLabels()
			assert.Loosely(t, l, should.HaveLength(1))
			assert.Loosely(t, l, should.ContainKey("key1"))
			assert.Loosely(t, l["key1"], should.Equal("value1"))
		})
	})

	ftt.Run("GetInstance", t, func(t *ftt.Test) {
		t.Run("empty", func(t *ftt.Test) {
			v := &VM{}
			i := v.GetInstance().Stable
			assert.Loosely(t, i.Disks, should.HaveLength(0))
			assert.Loosely(t, i.MachineType, should.BeEmpty)
			assert.Loosely(t, i.Metadata, should.BeNil)
			assert.Loosely(t, i.MinCpuPlatform, should.BeEmpty)
			assert.Loosely(t, i.NetworkInterfaces, should.HaveLength(0))
			assert.Loosely(t, i.Scheduling, should.BeNil)
			assert.Loosely(t, i.ServiceAccounts, should.BeNil)
			assert.Loosely(t, i.ShieldedInstanceConfig, should.BeNil)
			assert.Loosely(t, i.Tags, should.BeNil)
			assert.Loosely(t, i.Labels, should.BeNil)
			assert.Loosely(t, i.ForceSendFields, should.BeNil)
		})

		t.Run("non-empty", func(t *ftt.Test) {
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
			assert.Loosely(t, i.Disks, should.HaveLength(1))
			assert.Loosely(t, i.MachineType, should.Equal("type"))
			assert.Loosely(t, i.Metadata, should.BeNil)
			assert.Loosely(t, i.MinCpuPlatform, should.Equal("plat"))
			assert.Loosely(t, i.NetworkInterfaces, should.HaveLength(1))
			assert.Loosely(t, i.ServiceAccounts, should.BeNil)
			assert.Loosely(t, i.Scheduling, should.NotBeNil)
			assert.Loosely(t, i.Scheduling.NodeAffinities, should.HaveLength(1))
			assert.Loosely(t, i.ShieldedInstanceConfig, should.NotBeNil)
			assert.Loosely(t, i.Tags, should.BeNil)
			assert.Loosely(t, i.Labels, should.BeNil)
			assert.Loosely(t, i.ForceSendFields, should.NotBeNil)
			assert.Loosely(t, i.NullFields, should.NotBeNil)
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
