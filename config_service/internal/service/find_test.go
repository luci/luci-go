// Copyright 2023 The LUCI Authors.
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

package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/testutil"
)

func TestFinder(t *testing.T) {
	t.Parallel()

	ftt.Run("Finder", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		configSet := config.MustProjectSet("my-project")

		t.Run("Single service", func(t *ftt.Test) {
			const serviceName = "my-service"
			updateService := func(updateFn func(*model.Service)) {
				srv := &model.Service{
					Name: serviceName,
					Info: &cfgcommonpb.Service{
						Id: serviceName,
					},
				}
				updateFn(srv)
				assert.Loosely(t, datastore.Put(ctx, srv), should.BeNil)
			}

			t.Run("Exact match", func(t *ftt.Test) {
				for _, prefix := range []string{"exact:", "text:", ""} {
					t.Run(fmt.Sprintf("With prefix %q", prefix), func(t *ftt.Test) {
						updateService(func(srv *model.Service) {
							srv.Metadata = &cfgcommonpb.ServiceMetadata{
								ConfigPatterns: []*cfgcommonpb.ConfigPattern{
									{
										ConfigSet: prefix + string(configSet),
										Path:      prefix + "foo.cfg",
									},
								},
							}
						})
						finder, err := NewFinder(ctx)
						assert.Loosely(t, err, should.BeNil)
						services := finder.FindInterestedServices(ctx, configSet, "foo.cfg")
						assert.Loosely(t, convertToServiceNames(services), should.Resemble([]string{serviceName}))
						assert.Loosely(t, finder.FindInterestedServices(ctx, configSet, "boo.cfg"), should.BeEmpty)
					})
				}
			})

			t.Run("Regexp match", func(t *ftt.Test) {
				t.Run("Anchored", func(t *ftt.Test) {
					updateService(func(srv *model.Service) {
						srv.Metadata = &cfgcommonpb.ServiceMetadata{
							ConfigPatterns: []*cfgcommonpb.ConfigPattern{
								{
									ConfigSet: `regex:^projects/.+$`,
									Path:      `regex:^bucket\-[a-z]+\.cfg$`,
								},
							},
						}
					})
					finder, err := NewFinder(ctx)
					assert.Loosely(t, err, should.BeNil)
					services := finder.FindInterestedServices(ctx, configSet, "bucket-foo.cfg")
					assert.Loosely(t, convertToServiceNames(services), should.Resemble([]string{serviceName}))
					assert.Loosely(t, finder.FindInterestedServices(ctx, configSet, "bucket-foo1.cfg"), should.BeEmpty)
				})

				t.Run("Auto anchor", func(t *ftt.Test) {
					updateService(func(srv *model.Service) {
						srv.Metadata = &cfgcommonpb.ServiceMetadata{
							ConfigPatterns: []*cfgcommonpb.ConfigPattern{
								{
									ConfigSet: `regex:projects/.+`,
									Path:      `regex:bucket\-[a-z]+\.cfg`,
								},
							},
						}
					})
					finder, err := NewFinder(ctx)
					assert.Loosely(t, err, should.BeNil)
					services := finder.FindInterestedServices(ctx, configSet, "bucket-foo.cfg")
					assert.Loosely(t, convertToServiceNames(services), should.Resemble([]string{serviceName}))
					assert.Loosely(t, finder.FindInterestedServices(ctx, "abc"+configSet, "bucket-foo.cfg"), should.BeEmpty)
					assert.Loosely(t, finder.FindInterestedServices(ctx, configSet, "bucket-foo.cfg.gz"), should.BeEmpty)
				})
			})

			t.Run("Log warning if a service is interested in the config of another service", func(t *ftt.Test) {
				updateService(func(srv *model.Service) {
					srv.Metadata = &cfgcommonpb.ServiceMetadata{
						ConfigPatterns: []*cfgcommonpb.ConfigPattern{
							{
								ConfigSet: `regex:^services/.+$`,
								Path:      `regex:^settings\-[a-z]+\.cfg$`,
							},
						},
					}
				})
				ctx = memlogger.Use(ctx)
				logs := logging.Get(ctx).(*memlogger.MemLogger)
				finder, err := NewFinder(ctx)
				assert.Loosely(t, err, should.BeNil)
				services := finder.FindInterestedServices(ctx, config.MustServiceSet("not-my-service"), "settings-foo.cfg")
				assert.Loosely(t, services, should.NotBeEmpty)
				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Warning, fmt.Sprintf("crbug/1466976 - service %q declares it is interested in the config \"settings-foo.cfg\" of another service \"not-my-service\"", serviceName)))
			})
		})

		t.Run("Multiple services", func(t *ftt.Test) {
			services := []*model.Service{
				{
					Name: "buildbucket",
					Metadata: &cfgcommonpb.ServiceMetadata{
						ConfigPatterns: []*cfgcommonpb.ConfigPattern{
							{
								ConfigSet: `regex:projects/.+`,
								Path:      "exact:buildbucket.cfg",
							},
							{
								ConfigSet: `regex:projects/.+`,
								Path:      `regex:bucket-\w+.cfg`,
							},
						},
					},
				},
				{
					Name: "buildbucket-shadow",
					Metadata: &cfgcommonpb.ServiceMetadata{
						ConfigPatterns: []*cfgcommonpb.ConfigPattern{
							{
								ConfigSet: `regex:projects/.+`,
								Path:      "exact:buildbucket.cfg",
							},
							{
								ConfigSet: `regex:projects/.+`,
								Path:      `regex:bucket-\w+.cfg`,
							},
						},
					},
				},
				{
					Name: "luci-change-verifier",
					Metadata: &cfgcommonpb.ServiceMetadata{
						ConfigPatterns: []*cfgcommonpb.ConfigPattern{
							{
								ConfigSet: `regex:projects/.+`,
								Path:      "text:commit-queue.cfg",
							},
						},
					},
				},
				{
					Name: "swarming",
					LegacyMetadata: &cfgcommonpb.ServiceDynamicMetadata{
						Validation: &cfgcommonpb.Validator{
							Patterns: []*cfgcommonpb.ConfigPattern{
								{
									ConfigSet: `regex:projects/.+`,
									Path:      "swarming.cfg",
								},
							},
						},
					},
				},
				{
					Name: "buildbucket-project-foo-specific",
					Metadata: &cfgcommonpb.ServiceMetadata{
						ConfigPatterns: []*cfgcommonpb.ConfigPattern{
							{
								ConfigSet: "exact:projects/foo",
								Path:      "exact:buildbucket.cfg",
							},
						},
					},
				},
				{
					Name: "no-metadata",
				},
			}
			assert.Loosely(t, datastore.Put(ctx, services), should.BeNil)

			finder, err := NewFinder(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, convertToServiceNames(finder.FindInterestedServices(ctx, config.MustProjectSet("my-proj"), "bucket-abc.cfg")), should.Resemble([]string{"buildbucket", "buildbucket-shadow"}))
			assert.Loosely(t, convertToServiceNames(finder.FindInterestedServices(ctx, config.MustProjectSet("my-proj"), "buildbucket.cfg")), should.Resemble([]string{"buildbucket", "buildbucket-shadow"}))
			assert.Loosely(t, convertToServiceNames(finder.FindInterestedServices(ctx, config.MustProjectSet("my-proj"), "commit-queue.cfg")), should.Resemble([]string{"luci-change-verifier"}))
			assert.Loosely(t, convertToServiceNames(finder.FindInterestedServices(ctx, config.MustProjectSet("my-proj"), "swarming.cfg")), should.Resemble([]string{"swarming"}))
			assert.Loosely(t, convertToServiceNames(finder.FindInterestedServices(ctx, config.MustProjectSet("foo"), "buildbucket.cfg")), should.Resemble([]string{"buildbucket", "buildbucket-project-foo-specific", "buildbucket-shadow"}))
		})

		t.Run("Refresh", func(t *ftt.Test) {
			cs := config.MustProjectSet("my-proj")
			srv := &model.Service{
				Name: "foo",
				Metadata: &cfgcommonpb.ServiceMetadata{
					ConfigPatterns: []*cfgcommonpb.ConfigPattern{
						{
							ConfigSet: string(cs),
							Path:      "old.cfg",
						},
					},
				},
			}
			assert.Loosely(t, datastore.Put(ctx, srv), should.BeNil)
			finder, err := NewFinder(ctx)
			assert.Loosely(t, err, should.BeNil)
			cctx, cancel := context.WithCancel(ctx)
			defer cancel()
			go func() {
				finder.RefreshPeriodically(cctx)
			}()
			assert.Loosely(t, convertToServiceNames(finder.FindInterestedServices(ctx, cs, "old.cfg")), should.Resemble([]string{"foo"}))
			assert.Loosely(t, convertToServiceNames(finder.FindInterestedServices(ctx, cs, "new.cfg")), should.BeEmpty)
			srv.Metadata.ConfigPatterns[0].Path = "new.cfg"
			assert.Loosely(t, datastore.Put(ctx, srv), should.BeNil)
			tc := clock.Get(ctx).(testclock.TestClock)
			start := time.Now()
			for {
				tc.Add(1 * time.Minute) // Should trigger a refresh
				if len(finder.FindInterestedServices(ctx, cs, "new.cfg")) > 0 {
					break
				}
				if time.Since(start) > 30*time.Second {
					t.Fatal("finder doesn't appear to be refreshed")
				}
			}
			assert.Loosely(t, convertToServiceNames(finder.FindInterestedServices(ctx, cs, "old.cfg")), should.BeEmpty)
			assert.Loosely(t, convertToServiceNames(finder.FindInterestedServices(ctx, cs, "new.cfg")), should.Resemble([]string{"foo"}))
		})
	})
}

func convertToServiceNames(services []*model.Service) []string {
	if services == nil {
		return nil
	}
	ret := make([]string, len(services))
	for i, srv := range services {
		ret[i] = srv.Name
	}
	return ret
}
