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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/validation"
	gae "go.chromium.org/luci/gae/impl/memory"

	gce "go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/projects/v1"
	rpc "go.chromium.org/luci/gce/appengine/rpc/memory"
)

func TestDeref(t *testing.T) {
	t.Parallel()

	ftt.Run("deref", t, func(t *ftt.Test) {
		t.Run("empty", func(t *ftt.Test) {
			c := cfgclient.Use(gae.Use(context.Background()), memory.New(nil))
			cfg := &Config{}
			assert.Loosely(t, deref(c, cfg), should.BeNil)
			assert.Loosely(t, cfg.VMs.GetVms(), should.HaveLength(0))
		})

		t.Run("missing", func(t *ftt.Test) {
			c := cfgclient.Use(gae.Use(context.Background()), memory.New(nil))
			cfg := &Config{
				VMs: &gce.Configs{
					Vms: []*gce.Config{
						{
							Attributes: &gce.VM{
								Metadata: []*gce.Metadata{
									{
										Metadata: &gce.Metadata_FromFile{
											FromFile: "key:file",
										},
									},
								},
							},
						},
					},
				},
			}
			assert.Loosely(t, deref(c, cfg), should.ErrLike("failed to fetch"))
		})

		t.Run("revision", func(t *ftt.Test) {
			c := cfgclient.Use(gae.Use(context.Background()), memory.New(map[config.Set]memory.Files{
				"services/${appid}": map[string]string{
					"file": "val",
				},
			}))
			cfg := &Config{
				revision: "revision",
				VMs: &gce.Configs{
					Vms: []*gce.Config{
						{
							Attributes: &gce.VM{
								Metadata: []*gce.Metadata{
									{
										Metadata: &gce.Metadata_FromFile{
											FromFile: "key:file",
										},
									},
								},
							},
						},
					},
				},
			}
			assert.Loosely(t, deref(c, cfg), should.ErrLike("config revision mismatch"))
			assert.Loosely(t, cfg.VMs.Vms[0].Attributes.Metadata, should.ContainMatch(&gce.Metadata{
				Metadata: &gce.Metadata_FromFile{
					FromFile: "key:file",
				},
			}))
		})

		t.Run("dereferences", func(t *ftt.Test) {
			c := cfgclient.Use(gae.Use(context.Background()), memory.New(map[config.Set]memory.Files{
				"services/${appid}": map[string]string{
					"metadata/file": "val2",
				},
			}))
			cfg := &Config{
				revision: "12f8812df1e3182615a8f105db567f8d792a1440",
				VMs: &gce.Configs{
					Vms: []*gce.Config{
						{
							Attributes: &gce.VM{
								Metadata: []*gce.Metadata{
									{
										Metadata: &gce.Metadata_FromText{
											FromText: "key:val1",
										},
									},
									{
										Metadata: &gce.Metadata_FromFile{
											FromFile: "key:metadata/file",
										},
									},
								},
							},
						},
					},
				},
			}
			assert.Loosely(t, deref(c, cfg), should.BeNil)
			assert.Loosely(t, cfg.VMs.Vms[0].Attributes.Metadata, should.ContainMatch(&gce.Metadata{
				Metadata: &gce.Metadata_FromText{
					FromText: "key:val1",
				},
			}))
			assert.Loosely(t, cfg.VMs.Vms[0].Attributes.Metadata, should.ContainMatch(&gce.Metadata{
				Metadata: &gce.Metadata_FromText{
					FromText: "key:val2",
				},
			}))
		})
	})
}

func TestFetch(t *testing.T) {
	t.Parallel()

	ftt.Run("fetch", t, func(t *ftt.Test) {
		t.Run("invalid", func(t *ftt.Test) {
			t.Run("projects", func(t *ftt.Test) {
				c := cfgclient.Use(gae.Use(context.Background()), memory.New(map[config.Set]memory.Files{
					"services/${appid}": map[string]string{
						projectsFile: "invalid",
					},
				}))
				_, err := fetch(c)
				assert.Loosely(t, err, should.ErrLike("failed to load"))
			})

			t.Run("vms", func(t *ftt.Test) {
				c := cfgclient.Use(gae.Use(context.Background()), memory.New(map[config.Set]memory.Files{
					"services/${appid}": map[string]string{
						vmsFile: "invalid",
					},
				}))
				_, err := fetch(c)
				assert.Loosely(t, err, should.ErrLike("failed to load"))
			})
		})

		t.Run("empty", func(t *ftt.Test) {
			t.Run("missing", func(t *ftt.Test) {
				c := cfgclient.Use(gae.Use(context.Background()), memory.New(nil))
				cfg, err := fetch(c)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cfg.Projects, should.Match(&projects.Configs{}))
				assert.Loosely(t, cfg.VMs, should.Match(&gce.Configs{}))
			})

			t.Run("implicit", func(t *ftt.Test) {
				c := cfgclient.Use(gae.Use(context.Background()), memory.New(map[config.Set]memory.Files{
					"services/${appid}": {},
				}))
				cfg, err := fetch(c)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cfg.Projects, should.Match(&projects.Configs{}))
				assert.Loosely(t, cfg.VMs, should.Match(&gce.Configs{}))
			})

			t.Run("explicit", func(t *ftt.Test) {
				c := cfgclient.Use(gae.Use(context.Background()), memory.New(map[config.Set]memory.Files{
					"services/${appid}": {
						projectsFile: "",
						vmsFile:      "",
					},
				}))
				cfg, err := fetch(c)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cfg.Projects, should.Match(&projects.Configs{}))
				assert.Loosely(t, cfg.VMs, should.Match(&gce.Configs{}))
			})
		})
	})
}

func TestNormalize(t *testing.T) {
	t.Parallel()

	ftt.Run("normalize", t, func(t *ftt.Test) {
		c := context.Background()

		t.Run("empty", func(t *ftt.Test) {
			cfg := &Config{}
			assert.Loosely(t, normalize(c, cfg), should.BeNil)
			assert.Loosely(t, cfg.VMs.GetVms(), should.HaveLength(0))
		})

		t.Run("amount", func(t *ftt.Test) {
			cfg := &Config{
				VMs: &gce.Configs{
					Vms: []*gce.Config{
						{
							Amount: &gce.Amount{
								Change: []*gce.Schedule{
									{
										Length: &gce.TimePeriod{
											Time: &gce.TimePeriod_Duration{
												Duration: "1h",
											},
										},
									},
								},
							},
						},
					},
				},
			}
			assert.Loosely(t, normalize(c, cfg), should.BeNil)
			assert.Loosely(t, cfg.VMs.GetVms()[0].Amount.Change[0].Length.GetSeconds(), should.Equal(3600))
		})

		t.Run("lifetime", func(t *ftt.Test) {
			cfg := &Config{
				VMs: &gce.Configs{
					Vms: []*gce.Config{
						{
							Lifetime: &gce.TimePeriod{
								Time: &gce.TimePeriod_Duration{
									Duration: "1h",
								},
							},
						},
					},
				},
			}
			assert.Loosely(t, normalize(c, cfg), should.BeNil)
			assert.Loosely(t, cfg.VMs.GetVms()[0].Lifetime.GetSeconds(), should.Equal(3600))
		})

		t.Run("revision", func(t *ftt.Test) {
			cfg := &Config{
				revision: "revision",
				Projects: &projects.Configs{
					Project: []*projects.Config{
						{},
					},
				},
				VMs: &gce.Configs{
					Vms: []*gce.Config{
						{},
					},
				},
			}
			assert.Loosely(t, normalize(c, cfg), should.BeNil)
			assert.Loosely(t, cfg.Projects.GetProject()[0].Revision, should.Equal("revision"))
			assert.Loosely(t, cfg.VMs.GetVms()[0].Revision, should.Equal("revision"))
		})

		t.Run("timeout", func(t *ftt.Test) {
			cfg := &Config{
				VMs: &gce.Configs{
					Vms: []*gce.Config{
						{
							Timeout: &gce.TimePeriod{
								Time: &gce.TimePeriod_Duration{
									Duration: "1h",
								},
							},
						},
					},
				},
			}
			assert.Loosely(t, normalize(c, cfg), should.BeNil)
			assert.Loosely(t, cfg.VMs.GetVms()[0].Timeout.GetSeconds(), should.Equal(3600))
		})
	})
}

func TestSyncPrjs(t *testing.T) {
	t.Parallel()

	ftt.Run("syncPrjs", t, func(t *ftt.Test) {
		srv := &rpc.Projects{}
		c := withProjServer(context.Background(), srv)

		t.Run("nil", func(t *ftt.Test) {
			prjs := []*projects.Config{}
			assert.Loosely(t, syncPrjs(c, prjs), should.BeNil)
			rsp, err := srv.List(c, &projects.ListRequest{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rsp.Projects, should.BeEmpty)
		})

		t.Run("creates", func(t *ftt.Test) {
			prjs := []*projects.Config{
				{
					Project: "project",
				},
			}
			assert.Loosely(t, syncPrjs(c, prjs), should.BeNil)
			rsp, err := srv.List(c, &projects.ListRequest{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rsp.Projects, should.HaveLength(1))
			assert.Loosely(t, rsp.Projects[0].Project, should.Equal("project"))
		})

		t.Run("updates", func(t *ftt.Test) {
			srv.Ensure(c, &projects.EnsureRequest{
				Id: "project",
				Project: &projects.Config{
					Project: "project",
					Region: []string{
						"region1",
					},
					Revision: "revision-1",
				},
			})
			prjs := []*projects.Config{
				{
					Project: "project",
					Region: []string{
						"region2",
						"region3",
					},
					Revision: "revision-2",
				},
			}
			assert.Loosely(t, syncPrjs(c, prjs), should.BeNil)
			rsp, err := srv.List(c, &projects.ListRequest{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rsp.Projects, should.HaveLength(1))
			assert.Loosely(t, rsp.Projects, should.ContainMatch(&projects.Config{
				Project: "project",
				Region: []string{
					"region2",
					"region3",
				},
				Revision: "revision-2",
			}))
		})

		t.Run("deletes", func(t *ftt.Test) {
			srv.Ensure(c, &projects.EnsureRequest{
				Id: "project",
				Project: &projects.Config{
					Project: "project",
				},
			})
			assert.Loosely(t, syncPrjs(c, nil), should.BeNil)
			rsp, err := srv.List(c, &projects.ListRequest{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rsp.Projects, should.BeEmpty)
		})
	})
}

func TestSyncVMs(t *testing.T) {
	t.Parallel()

	ftt.Run("syncVMs", t, func(t *ftt.Test) {
		srv := &rpc.Config{}
		c := withVMsServer(context.Background(), srv)

		t.Run("nil", func(t *ftt.Test) {
			vms := []*gce.Config{}
			assert.Loosely(t, syncVMs(c, vms), should.BeNil)
			rsp, err := srv.List(c, &gce.ListRequest{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rsp.Configs, should.BeEmpty)
		})

		t.Run("creates", func(t *ftt.Test) {
			vms := []*gce.Config{
				{
					Prefix: "prefix",
				},
			}
			assert.Loosely(t, syncVMs(c, vms), should.BeNil)
			rsp, err := srv.List(c, &gce.ListRequest{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rsp.Configs, should.HaveLength(1))
			assert.Loosely(t, rsp.Configs[0].Prefix, should.Equal("prefix"))
		})

		t.Run("updates", func(t *ftt.Test) {
			srv.Ensure(c, &gce.EnsureRequest{
				Id: "prefix",
				Config: &gce.Config{
					Amount: &gce.Amount{
						Min: 1,
						Max: 2,
					},
					Prefix:   "prefix",
					Revision: "revision-1",
				},
			})
			vms := []*gce.Config{
				{
					Amount: &gce.Amount{
						Min: 2,
						Max: 3,
					},
					Prefix:   "prefix",
					Revision: "revision-2",
				},
			}
			assert.Loosely(t, syncVMs(c, vms), should.BeNil)
			rsp, err := srv.List(c, &gce.ListRequest{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rsp.Configs, should.HaveLength(1))
			assert.Loosely(t, rsp.Configs, should.ContainMatch(&gce.Config{
				Amount: &gce.Amount{
					Min: 2,
					Max: 3,
				},
				Prefix:   "prefix",
				Revision: "revision-2",
			}))
		})

		t.Run("deletes", func(t *ftt.Test) {
			srv.Ensure(c, &gce.EnsureRequest{
				Id: "prefix",
				Config: &gce.Config{
					Prefix: "prefix",
				},
			})
			assert.Loosely(t, syncVMs(c, nil), should.BeNil)
			rsp, err := srv.List(c, &gce.ListRequest{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rsp.Configs, should.BeEmpty)
		})
	})
}

func TestValidate(t *testing.T) {
	t.Parallel()

	ftt.Run("validate", t, func(t *ftt.Test) {
		c := context.Background()

		t.Run("invalid", func(t *ftt.Test) {
			t.Run("projects", func(t *ftt.Test) {
				cfg := &Config{
					Projects: &projects.Configs{
						Project: []*projects.Config{
							{},
						},
					},
				}
				assert.Loosely(t, validate(c, cfg), should.ErrLike("is required"))
			})

			t.Run("vms", func(t *ftt.Test) {
				cfg := &Config{
					VMs: &gce.Configs{
						Vms: []*gce.Config{
							{},
						},
					},
				}
				assert.Loosely(t, validate(c, cfg), should.ErrLike("is required"))
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				cfg := &Config{}
				err := validate(c, cfg)
				assert.Loosely(t, err, should.BeNil)
			})
		})
	})
}

func TestValidateProjectsConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("validate projects.cfg", t, func(t *ftt.Test) {
		vctx := &validation.Context{
			Context: context.Background(),
		}
		configSet := "services/${appid}"
		path := "projects.cfg"

		t.Run("bad proto", func(t *ftt.Test) {
			content := []byte(` bad: "bad" `)
			assert.Loosely(t, validateProjectsCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("invalid ProjectsCfg proto message"))
		})

		t.Run("valid proto", func(t *ftt.Test) {
			content := []byte(` `)
			assert.Loosely(t, validateProjectsCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})
	})
}

func TestValidateVMsConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("validate vms.cfg", t, func(t *ftt.Test) {
		vctx := &validation.Context{
			Context: context.Background(),
		}
		configSet := "services/${appid}"
		path := "vms.cfg"

		t.Run("bad proto", func(t *ftt.Test) {
			content := []byte(` bad: "bad" `)
			assert.Loosely(t, validateVMsCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("invalid VMsCfg proto message"))
		})

		t.Run("valid proto", func(t *ftt.Test) {
			content := []byte(`
				vms {
					amount {
						min: 1
						max: 1
					}
					attributes {
						network_interface {
							access_config {
							}
						}
						disk {
							image: "global/images/chrome-trusty-v1"
						}
						metadata {
							from_file: "cipd_deployments:metadata/file.json"
						}
						metadata {
							from_text: "enable-oslogin:TRUE"
						}
						machine_type: "zones/{{.Zone}}/machineTypes/n1-standard-2"
						project: "google.com:chromecompute"
						zone: "us-central1-b"
					}
					lifetime {
						duration: "1d"
					}
					prefix: "chrome-trusty"
				}
			`)
			assert.Loosely(t, validateVMsCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})
	})
}
