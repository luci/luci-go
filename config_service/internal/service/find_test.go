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
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFinder(t *testing.T) {
	t.Parallel()

	Convey("Finder", t, func() {
		ctx := testutil.SetupContext()
		configSet := config.MustProjectSet("my-project")

		Convey("Single service", func() {
			const serviceName = "my-service"
			updateService := func(updateFn func(*model.Service)) {
				srv := &model.Service{
					Name: serviceName,
					Info: &cfgcommonpb.Service{
						Id: serviceName,
					},
				}
				updateFn(srv)
				So(datastore.Put(ctx, srv), ShouldBeNil)
			}

			Convey("Exact match", func() {
				for _, prefix := range []string{"exact:", "text:", ""} {
					Convey(fmt.Sprintf("With prefix %q", prefix), func() {
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
						So(err, ShouldBeNil)
						services := finder.FindInterestedServices(configSet, "foo.cfg")
						So(convertToServiceNames(services), ShouldResemble, []string{serviceName})
						So(finder.FindInterestedServices(configSet, "boo.cfg"), ShouldBeEmpty)
					})
				}
			})

			Convey("Regexp match", func() {
				Convey("Anchored", func() {
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
					So(err, ShouldBeNil)
					services := finder.FindInterestedServices(configSet, "bucket-foo.cfg")
					So(convertToServiceNames(services), ShouldResemble, []string{serviceName})
					So(finder.FindInterestedServices(configSet, "bucket-foo1.cfg"), ShouldBeEmpty)
				})

				Convey("Auto anchor", func() {
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
					So(err, ShouldBeNil)
					services := finder.FindInterestedServices(configSet, "bucket-foo.cfg")
					So(convertToServiceNames(services), ShouldResemble, []string{serviceName})
					So(finder.FindInterestedServices("abc"+configSet, "bucket-foo.cfg"), ShouldBeEmpty)
					So(finder.FindInterestedServices(configSet, "bucket-foo.cfg.gz"), ShouldBeEmpty)
				})
			})
		})

		Convey("Multiple services", func() {
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
			So(datastore.Put(ctx, services), ShouldBeNil)

			finder, err := NewFinder(ctx)
			So(err, ShouldBeNil)
			So(convertToServiceNames(finder.FindInterestedServices(config.MustProjectSet("my-proj"), "bucket-abc.cfg")), ShouldResemble, []string{"buildbucket", "buildbucket-shadow"})
			So(convertToServiceNames(finder.FindInterestedServices(config.MustProjectSet("my-proj"), "buildbucket.cfg")), ShouldResemble, []string{"buildbucket", "buildbucket-shadow"})
			So(convertToServiceNames(finder.FindInterestedServices(config.MustProjectSet("my-proj"), "commit-queue.cfg")), ShouldResemble, []string{"luci-change-verifier"})
			So(convertToServiceNames(finder.FindInterestedServices(config.MustProjectSet("my-proj"), "swarming.cfg")), ShouldResemble, []string{"swarming"})
			So(convertToServiceNames(finder.FindInterestedServices(config.MustProjectSet("foo"), "buildbucket.cfg")), ShouldResemble, []string{"buildbucket", "buildbucket-project-foo-specific", "buildbucket-shadow"})
		})

		Convey("Refresh", func() {
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
			So(datastore.Put(ctx, srv), ShouldBeNil)
			finder, err := NewFinder(ctx)
			So(err, ShouldBeNil)
			cctx, cancel := context.WithCancel(ctx)
			defer cancel()
			go func() {
				finder.RefreshPeriodically(cctx)
			}()
			So(convertToServiceNames(finder.FindInterestedServices(cs, "old.cfg")), ShouldResemble, []string{"foo"})
			So(convertToServiceNames(finder.FindInterestedServices(cs, "new.cfg")), ShouldBeEmpty)
			srv.Metadata.ConfigPatterns[0].Path = "new.cfg"
			So(datastore.Put(ctx, srv), ShouldBeNil)
			tc := clock.Get(ctx).(testclock.TestClock)
			start := time.Now()
			for {
				tc.Add(1 * time.Minute) // Should trigger a refresh
				if len(finder.FindInterestedServices(cs, "new.cfg")) > 0 {
					break
				}
				if time.Since(start) > 30*time.Second {
					t.Fatal("finder doesn't appear to be refreshed")
				}
			}
			So(convertToServiceNames(finder.FindInterestedServices(cs, "old.cfg")), ShouldBeEmpty)
			So(convertToServiceNames(finder.FindInterestedServices(cs, "new.cfg")), ShouldResemble, []string{"foo"})
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
