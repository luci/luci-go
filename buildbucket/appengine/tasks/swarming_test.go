// Copyright 2022 The LUCI Authors.
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

package tasks

import (
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/api/swarming/swarming/v1"

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/cmd/bbagent/bbinput"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTaskDef(t *testing.T) {
	Convey("compute task slice", t, func() {
		b := &model.Build{
			ID: 123,
			Proto: &pb.Build{
				Id: 123,
				SchedulingTimeout: &durationpb.Duration{
					Seconds: 3600,
				},
				ExecutionTimeout: &durationpb.Duration{
					Seconds: 4800,
				},
				GracePeriod: &durationpb.Duration{
					Seconds: 60,
				},
				Infra: &pb.BuildInfra{
					Swarming: &pb.BuildInfra_Swarming{},
					Bbagent: &pb.BuildInfra_BBAgent{
						CacheDir:    "cache",
						PayloadPath: "kitchen-checkout",
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Agent: &pb.BuildInfra_Buildbucket_Agent{
							Source: &pb.BuildInfra_Buildbucket_Agent_Source{
								DataType: &pb.BuildInfra_Buildbucket_Agent_Source_Cipd{
									Cipd: &pb.BuildInfra_Buildbucket_Agent_Source_CIPD{
										Package: "infra/tools/luci/bbagent/${platform}",
										Version: "canary-version",
										Server:  "cipd server",
									},
								},
							},
						},
					},
				},
			},
		}
		Convey("only base slice", func() {
			b.Proto.Infra.Swarming = &pb.BuildInfra_Swarming{
				Caches: []*pb.BuildInfra_Swarming_CacheEntry{
					{Name: "shared_builder_cache", Path: "builder"},
				},
				TaskDimensions: []*pb.RequestedDimension{
					{Key: "pool", Value: "Chrome"},
				},
			}
			slices, err := computeTaskSlice(b)
			So(err, ShouldBeNil)
			So(len(slices), ShouldEqual, 1)
			So(slices[0].Properties.Caches, ShouldResemble, []*swarming.SwarmingRpcsCacheEntry{{
				Path: filepath.Join("cache", "builder"),
				Name: "shared_builder_cache",
			}})
			So(slices[0].Properties.Dimensions, ShouldResemble, []*swarming.SwarmingRpcsStringPair{
				{
					Key:   "caches",
					Value: "shared_builder_cache",
				},
				{
					Key:   "pool",
					Value: "Chrome",
				},
			})
		})

		Convey("multiple dimensions and cache fallback", func() {
			// Creates 4 task_slices by modifying the buildercfg in 2 ways:
			//  - Add two named caches, one expiring at 60 seconds, one at 360 seconds.
			//  - Add an optional builder dimension, expiring at 120 seconds.
			//
			// This ensures the combination of these features works correctly, and that
			// multiple 'caches' dimensions can be injected.
			b.Proto.Infra.Swarming = &pb.BuildInfra_Swarming{
				Caches: []*pb.BuildInfra_Swarming_CacheEntry{
					{Name: "shared_builder_cache", Path: "builder", WaitForWarmCache: &durationpb.Duration{Seconds: 60}},
					{Name: "second_cache", Path: "second", WaitForWarmCache: &durationpb.Duration{Seconds: 360}},
				},
				TaskDimensions: []*pb.RequestedDimension{
					{Key: "a", Value: "1", Expiration: &durationpb.Duration{Seconds: 120}},
					{Key: "a", Value: "2", Expiration: &durationpb.Duration{Seconds: 120}},
					{Key: "pool", Value: "Chrome"},
				},
			}
			slices, err := computeTaskSlice(b)
			So(err, ShouldBeNil)
			So(len(slices), ShouldEqual, 4)

			// All slices properties fields have the same value except dimensions.
			for _, tSlice := range slices {
				So(tSlice.Properties.ExecutionTimeoutSecs, ShouldEqual, 4800)
				So(tSlice.Properties.GracePeriodSecs, ShouldEqual, 240)
				So(tSlice.Properties.Caches, ShouldResemble, []*swarming.SwarmingRpcsCacheEntry{
					{Path: filepath.Join("cache", "builder"), Name: "shared_builder_cache"},
					{Path: filepath.Join("cache", "second"), Name: "second_cache"},
				})
				So(tSlice.Properties.Env, ShouldResemble, []*swarming.SwarmingRpcsStringPair{
					{Key: "BUILDBUCKET_EXPERIMENTAL", Value: "FALSE"},
				})
			}

			So(slices[0].ExpirationSecs, ShouldEqual, 60)
			// The dimensions are different. 'a' and 'caches' are injected.
			So(slices[0].Properties.Dimensions, ShouldResemble, []*swarming.SwarmingRpcsStringPair{
				{Key: "a", Value: "1"},
				{Key: "a", Value: "2"},
				{Key: "caches", Value: "second_cache"},
				{Key: "caches", Value: "shared_builder_cache"},
				{Key: "pool", Value: "Chrome"},
			})

			// 120 - 60
			So(slices[1].ExpirationSecs, ShouldEqual, 60)
			// The dimensions are different. 'a' and 'caches' are injected.
			So(slices[1].Properties.Dimensions, ShouldResemble, []*swarming.SwarmingRpcsStringPair{
				{Key: "a", Value: "1"},
				{Key: "a", Value: "2"},
				{Key: "caches", Value: "second_cache"},
				{Key: "pool", Value: "Chrome"},
			})

			// 360 - 120
			So(slices[2].ExpirationSecs, ShouldEqual, 240)
			// 'a' expired, one 'caches' remains.
			So(slices[2].Properties.Dimensions, ShouldResemble, []*swarming.SwarmingRpcsStringPair{
				{Key: "caches", Value: "second_cache"},
				{Key: "pool", Value: "Chrome"},
			})

			// 3600-360
			So(slices[3].ExpirationSecs, ShouldEqual, 3240)
			// # The cold fallback; the last 'caches' expired.
			So(slices[3].Properties.Dimensions, ShouldResemble, []*swarming.SwarmingRpcsStringPair{
				{Key: "pool", Value: "Chrome"},
			})
		})
	})

	Convey("compute bbagent command", t, func() {
		b := &model.Build{
			ID: 123,
			Proto: &pb.Build{
				Infra: &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "bbhost.com",
					},
				},
			},
		}
		Convey("bbagent_getbuild experiment", func() {
			b.Experiments = []string{"luci.buildbucket.bbagent_getbuild"}
			bbagentCmd := computeCommand(b)
			So(bbagentCmd, ShouldResemble, []string{
				"bbagent${EXECUTABLE_SUFFIX}",
				"-host",
				"bbhost.com",
				"-build-id",
				"123",
			})
		})

		Convey("no bbagent_getbuild experiment", func() {
			b.Proto.Infra.Bbagent = &pb.BuildInfra_BBAgent{
				CacheDir:    "cache",
				PayloadPath: "payload_path",
			}
			bbagentCmd := computeCommand(b)
			expectedEncoded := bbinput.Encode(&pb.BBAgentArgs{
				Build:       b.Proto,
				CacheDir:    "cache",
				PayloadPath: "payload_path",
			})
			So(bbagentCmd, ShouldResemble, []string{
				"bbagent${EXECUTABLE_SUFFIX}",
				expectedEncoded,
			})
		})
	})

	Convey("compute env_prefixes", t, func() {
		b := &model.Build{
			ID: 123,
			Proto: &pb.Build{
				Infra: &pb.BuildInfra{
					Swarming: &pb.BuildInfra_Swarming{},
				},
			},
		}
		Convey("empty swarming cach", func() {
			prefixes := computeEnvPrefixes(b)
			So(prefixes, ShouldResemble, []*swarming.SwarmingRpcsStringListPair{})
		})

		Convey("normal", func() {
			b.Proto.Infra.Swarming.Caches = []*pb.BuildInfra_Swarming_CacheEntry{
				{Path: "vpython", Name: "vpython", EnvVar: "VPYTHON_VIRTUALENV_ROOT"},
				{Path: "abc", Name: "abc", EnvVar: "ABC"},
			}
			prefixes := computeEnvPrefixes(b)
			So(prefixes, ShouldResemble, []*swarming.SwarmingRpcsStringListPair{
				{Key: "ABC", Value: []string{filepath.Join("cache", "abc")}},
				{Key: "VPYTHON_VIRTUALENV_ROOT", Value: []string{filepath.Join("cache", "vpython")}},
			})
		})
	})
}
