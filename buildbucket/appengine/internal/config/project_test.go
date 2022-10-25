// Copyright 2021 The LUCI Authors.
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
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateProject(t *testing.T) {
	t.Parallel()

	Convey("validate buildbucket cfg", t, func() {
		vctx := &validation.Context{
			Context: memory.Use(context.Background()),
		}
		configSet := "projects/test"
		path := "cr-buildbucket.cfg"
		settingsCfg := &pb.SettingsCfg{}
		So(SetTestSettingsCfg(vctx.Context, settingsCfg), ShouldBeNil)

		Convey("OK", func() {
			var okCfg = `
				buckets {
					name: "good.name"
				}
				buckets {
					name: "good.name2"
				}
			`
			So(validateProjectCfg(vctx, configSet, path, []byte(okCfg)), ShouldBeNil)
			So(vctx.Finalize(), ShouldBeNil)
		})

		Convey("bad proto", func() {
			content := []byte(` bad: "bad" `)
			So(validateProjectCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "invalid BuildbucketCfg proto message")
		})

		Convey("empty cr-buildbucket.cfg", func() {
			content := []byte(` `)
			So(validateProjectCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldBeNil)
		})

		Convey("fail", func() {
			var badCfg = `
				buckets {
					name: "a"
				}
				buckets {
					name: "a"
				}
				buckets {}
				buckets { name: "luci.x" }
			`
			So(validateProjectCfg(vctx, configSet, path, []byte(badCfg)), ShouldBeNil)
			ve, ok := vctx.Finalize().(*validation.Error)
			So(ok, ShouldEqual, true)
			So(len(ve.Errors), ShouldEqual, 3)
			So(ve.Errors[0].Error(), ShouldContainSubstring, "(buckets #1 - a): duplicate bucket name \"a\"")
			So(ve.Errors[1].Error(), ShouldContainSubstring, "(buckets #2 - ): invalid name \"\": bucket name is not specified")
			So(ve.Errors[2].Error(), ShouldContainSubstring, "(buckets #3 - luci.x): invalid name \"luci.x\": must start with 'luci.test.' because it starts with 'luci.' and is defined in the \"test\" project")
		})

		Convey("buckets unsorted", func() {
			badCfg := `
				buckets { name: "c" }
				buckets { name: "b" }
				buckets { name: "a" }
			`
			So(validateProjectCfg(vctx, configSet, path, []byte(badCfg)), ShouldBeNil)
			ve, ok := vctx.Finalize().(*validation.Error)
			So(ok, ShouldEqual, true)
			warnings := ve.WithSeverity(validation.Warning).(errors.MultiError)
			So(warnings[0].Error(), ShouldContainSubstring, "bucket \"b\" out of order")
			So(warnings[1].Error(), ShouldContainSubstring, "bucket \"a\" out of order")
		})
	})

	Convey("validate project_config.Swarming", t, func() {
		vctx := &validation.Context{
			Context: context.Background(),
		}
		wellKnownExperiments := stringset.NewFromSlice("luci.well_known")
		toBBSwarmingCfg := func(content string) *pb.Swarming {
			cfg := pb.Swarming{}
			err := prototext.Unmarshal([]byte(content), &cfg)
			So(err, ShouldBeNil)
			return &cfg
		}
		Convey("OK", func() {
			content := `
				builders {
					name: "release"
					swarming_host: "example.com"
					dimensions: "os:Linux"
					dimensions: "cpu:x86-64"
					dimensions: "cores:8"
					dimensions: "60:cores:64"
					service_account: "robot@example.com"
					caches {
						name: "git_chromium"
						path: "git_cache"
					}
					recipe {
						name: "foo"
						cipd_package: "infra/recipe_bundle"
						cipd_version: "refs/heads/main"
						properties: "a:b'"
						properties_j: "x:true"
					}
				}
				builders {
					name: "custom exe"
					swarming_host: "example.com"
					dimensions: "os:Linux"
					service_account: "robot@example.com"
					caches {
						name: "git_chromium"
						path: "git_cache"
					}
					exe {
						cipd_package: "infra/executable/foo"
						cipd_version: "refs/heads/main"
					}
					properties: '{"a":"b","x":true}'
					resultdb {
						enable: true
						history_options {
							use_invocation_timestamp: true
						}
					}
				}
				builders {
					name: "another custom exe"
					swarming_host: "example.com"
					dimensions: "os:Linux"
					service_account: "robot@example.com"
					caches {
						name: "git_chromium"
						path: "git_cache"
					}
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
				}
				builders {
					name: "release cipd"
					swarming_host: "example.com"
					recipe {
						cipd_package: "some/package"
						name: "foo"
					}
				}
			`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments)
			So(vctx.Finalize(), ShouldBeNil)
		})

		Convey("empty builders", func() {
			content := `builders {}`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments)
			ve, ok := vctx.Finalize().(*validation.Error)
			So(ok, ShouldEqual, true)
			So(len(ve.Errors), ShouldEqual, 3)
			So(ve.Errors[0].Error(), ShouldContainSubstring, "(swarming / builders #0 - ): name must match "+builderRegex.String())
			So(ve.Errors[1].Error(), ShouldContainSubstring, "(swarming / builders #0 - ): swarming_host unspecified")
			So(ve.Errors[2].Error(), ShouldContainSubstring, "(swarming / builders #0 - ): exactly one of exe or recipe must be specified")
		})

		Convey("bad builders cfg 1", func() {
			content := `
				builders {
					name: "both"
					swarming_host: "example.com"
					exe {
						cipd_package: "infra/executable"
					}
					recipe {
						name: "foo"
						cipd_package: "infra/recipe_bundle"
					}
				}
				builders {
					name: "bad exe"
					swarming_host: "example.com"
					exe {
						cipd_version: "refs/heads/main"
					}
				}
				builders {
					name: "non json properties"
					swarming_host: "example.com"
					exe {
						cipd_package: "infra/executable"
					}
					properties: "{1:2}"
				}
				builders {
					name: "non dict properties"
					swarming_host: "example.com"
					exe {
						cipd_package: "infra/executable"
					}
					properties: "[]"
				}
				builders {
					name: "bad recipe"
					swarming_host: "example.com"
					recipe {
						cipd_version: "refs/heads/master"
					}
				}
				builders {
					name: "recipe and properties"
					swarming_host: "example.com"
					recipe {
						name: "foo"
						cipd_package: "infra/recipe_bundle"
					}
					properties: "{}"
				}
				builders {
					name: "very_long_name_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
								"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
								"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
					swarming_host: "example.com"
					recipe {
						cipd_package: "some/package"
						name: "foo"
					}
				}
				builders {
					name: "bad resultdb"
					swarming_host: "example.com"
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
					resultdb {
						enable: true
						history_options {
							commit{
								position: 123
							}
						}
					}
				}
			`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments)
			ve, ok := vctx.Finalize().(*validation.Error)
			So(ok, ShouldEqual, true)
			So(len(ve.Errors), ShouldEqual, 9)
			So(ve.Errors[0].Error(), ShouldContainSubstring, "(swarming / builders #0 - both): exactly one of exe or recipe must be specified")
			So(ve.Errors[1].Error(), ShouldContainSubstring, "(swarming / builders #1 - bad exe): exe.cipd_package: unspecified")
			So(ve.Errors[2].Error(), ShouldContainSubstring, "(swarming / builders #2 - non json properties): properties is not a JSON object")
			So(ve.Errors[3].Error(), ShouldContainSubstring, "(swarming / builders #3 - non dict properties): properties is not a JSON object")
			So(ve.Errors[4].Error(), ShouldContainSubstring, "(swarming / builders #4 - bad recipe / recipe): name: unspecified")
			So(ve.Errors[5].Error(), ShouldContainSubstring, "(swarming / builders #4 - bad recipe / recipe): cipd_package: unspecified")
			So(ve.Errors[6].Error(), ShouldContainSubstring, "(swarming / builders #5 - recipe and properties): recipe and properties cannot be set together")
			So(ve.Errors[7].Error(), ShouldContainSubstring, "name must match ^[a-zA-Z0-9\\-_.\\(\\) ]{1,128}$")
			So(ve.Errors[8].Error(), ShouldContainSubstring, "(swarming / builders #7 - bad resultdb): resultdb.history_options.commit must be unset")
		})

		Convey("bad builders cfg 2", func() {
			content := `
				task_template_canary_percentage { value: 102 }
				builders {
					name: "meep"
					swarming_host: "example.com"
					recipe {
						cipd_package: "some/package"
						name: "foo"
					}
				}
				builders {
					name: "meep"
					swarming_host: "example.com"
					recipe {
						cipd_package: "some/package"
						name: "foo"
					}
					swarming_tags: "wrong"
				}
				builders {
					name: "another"
					swarming_host: "example.com"
					service_account: "not an email"
					recipe {
						cipd_package: "some/package"
						name: "foo"
					}
					priority: 300
				}
			`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments)
			ve, ok := vctx.Finalize().(*validation.Error)
			So(ok, ShouldEqual, true)
			So(len(ve.Errors), ShouldEqual, 5)
			So(ve.Errors[0].Error(), ShouldContainSubstring, "task_template_canary_percentage.value must must be in [0, 100]")
			So(ve.Errors[1].Error(), ShouldContainSubstring, "(swarming / builders #1 - meep / swarming_tags #0): Deprecated. Used only to enable \"vpython:native-python-wrapper\"")
			So(ve.Errors[2].Error(), ShouldContainSubstring, "(swarming / builders #1 - meep): name: duplicate")
			So(ve.Errors[3].Error(), ShouldContainSubstring, "priority: must be in [20, 255] range; got 300")
			So(ve.Errors[4].Error(), ShouldContainSubstring, `service_account "not an email" doesn't match "^[0-9a-zA-Z_\\-\\.\\+\\%]+@[0-9a-zA-Z_\\-\\.]+$"`)
		})

		Convey("bad caches in builders cfg", func() {
			content := `
				builders {
					name: "b1"
					swarming_host: "example.com"
					caches {}
					caches { name: "a/b" path: "a" }
					caches { name: "b" path: "a\\c" }
					caches { name: "c" path: "a/.." }
					caches { name: "d" path: "/a" }
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
				}
				builders {
					name: "rel"
					swarming_host: "swarming.example.com"
					caches { path: "a" name: "a" }
					caches { path: "a" name: "a" }
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
				}
				builders {
					name: "bad secs"
					swarming_host: "swarming.example.com"
					caches { path: "aa" name: "aa" wait_for_warm_cache_secs: 61 }
					caches { path: "bb" name: "bb" wait_for_warm_cache_secs: 59 }
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
				}
				builders {
					name: "many"
					swarming_host: "swarming.example.com"
					caches { path: "a" name: "a" wait_for_warm_cache_secs: 60 }
					caches { path: "b" name: "b" wait_for_warm_cache_secs: 120 }
					caches { path: "c" name: "c" wait_for_warm_cache_secs: 180 }
					caches { path: "d" name: "d" wait_for_warm_cache_secs: 240 }
					caches { path: "e" name: "e" wait_for_warm_cache_secs: 300 }
					caches { path: "f" name: "f" wait_for_warm_cache_secs: 360 }
					caches { path: "g" name: "g" wait_for_warm_cache_secs: 420 }
					caches { path: "h" name: "h" wait_for_warm_cache_secs: 480 }
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
				}
			`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments)
			ve, ok := vctx.Finalize().(*validation.Error)
			So(ok, ShouldEqual, true)
			So(len(ve.Errors), ShouldEqual, 11)
			So(ve.Errors[0].Error(), ShouldContainSubstring, "(swarming / builders #0 - b1 / caches #0): name: required")
			So(ve.Errors[1].Error(), ShouldContainSubstring, "(swarming / builders #0 - b1 / caches #0 / path): required")
			So(ve.Errors[2].Error(), ShouldContainSubstring, `(swarming / builders #0 - b1 / caches #1): name: "a/b" does not match "^[a-z0-9_]+$"`)
			So(ve.Errors[3].Error(), ShouldContainSubstring, `(swarming / builders #0 - b1 / caches #2 / path): cannot contain \. On Windows forward-slashes will be replaced with back-slashes.`)
			So(ve.Errors[4].Error(), ShouldContainSubstring, "(swarming / builders #0 - b1 / caches #3 / path): cannot contain '..'")
			So(ve.Errors[5].Error(), ShouldContainSubstring, "(swarming / builders #0 - b1 / caches #4 / path): cannot start with '/'")
			So(ve.Errors[6].Error(), ShouldContainSubstring, "(swarming / builders #1 - rel / caches #1): duplicate name")
			So(ve.Errors[7].Error(), ShouldContainSubstring, "(swarming / builders #1 - rel / caches #1): duplicate path")
			So(ve.Errors[8].Error(), ShouldContainSubstring, "(swarming / builders #2 - bad secs / caches #0): wait_for_warm_cache_secs must be rounded on 60 seconds")
			So(ve.Errors[9].Error(), ShouldContainSubstring, "(swarming / builders #2 - bad secs / caches #1): wait_for_warm_cache_secs must be at least 60 seconds")
			So(ve.Errors[10].Error(), ShouldContainSubstring, "(swarming / builders #3 - many): 'too many different (8) wait_for_warm_cache_secs values; max 7")
		})

		Convey("bad experiments in builders cfg", func() {
			content := `
				builders {
					name: "b1"
					swarming_host: "example.com"
					experiments {
						key: "bad!"
						value: 105
					}
					experiments {
						key: "negative"
						value: -10
					}
					experiments {
						key: "my.cool.experiment"
						value: 10
					}
					experiments {
						key: "luci.bad"
						value: 10
					}
					experiments {
						key: "luci.well_known"
						value: 10
					}
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
				}
			`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments)
			ve, ok := vctx.Finalize().(*validation.Error)
			So(ok, ShouldEqual, true)
			So(len(ve.Errors), ShouldEqual, 4)
			// Have to concatenate all error strings because experiments is Map and iteration over Map is non-deterministic.
			allErrs := fmt.Sprintf("%s\n%s\n%s\n%s", ve.Errors[0].Error(), ve.Errors[1].Error(), ve.Errors[2].Error(), ve.Errors[3].Error())
			So(allErrs, ShouldContainSubstring, `(swarming / builders #0 - b1 / experiments "bad!"): does not match "^[a-z][a-z0-9_]*(?:\\.[a-z][a-z0-9_]*)*$"`)
			So(allErrs, ShouldContainSubstring, `(swarming / builders #0 - b1 / experiments "bad!"): value must be in [0, 100]`)
			So(allErrs, ShouldContainSubstring, `(swarming / builders #0 - b1 / experiments "negative"): value must be in [0, 100]`)
			So(allErrs, ShouldContainSubstring, `(swarming / builders #0 - b1 / experiments "luci.bad"): unknown experiment has reserved prefix "luci."`)
		})
	})

	Convey("validate dimensions", t, func() {
		helper := func(expectedErr string, dimensions []string) {
			vctx := &validation.Context{
				Context: context.Background(),
			}
			validateDimensions(vctx, dimensions)
			if strings.HasPrefix(expectedErr, "ok") {
				So(vctx.Finalize(), ShouldBeNil)
			} else {
				So(vctx.Finalize().Error(), ShouldContainSubstring, expectedErr)
			}
		}

		testData := map[string][]string{
			"ok1": {"a:b"},
			"ok2": {"a:b1", "a:b2", "60:a:b3"},
			`ok3`: {"1814400:a:1"}, // 21*24*60*6
			`expiration_secs is outside valid range; up to 504h0m0s`:                                                     {"1814401:a:1"}, // 21*24*60*60+
			`(dimensions #0 - ""): "" does not have ':'`:                                                                 {""},
			`(dimensions #0 - "caches:a"): dimension key must not be 'caches'; caches must be declared via caches field`: {"caches:a"},
			`(dimensions #0 - ":"): missing key`:                                                                         {":"},
			`(dimensions #0 - "a.b:c"): key "a.b" does not match pattern "^[a-zA-Z\\_\\-]+$"`:                            {"a.b:c"},
			`(dimensions #0 - "0:"): missing key`:                                                                        {"0:"},
			`(dimensions #0 - "a:"): missing value`:                                                                      {"a:", "60:a:b"},
			`(dimensions #0 - "-1:a:1"): expiration_secs is outside valid range; up to 504h0m0s`:                         {"-1:a:1"},
			`(dimensions #0 - "1:a:b"): expiration_secs must be a multiple of 60 seconds`:                                {"1:a:b"},
			"at most 6 different expiration_secs values can be used": {
				"60:a:1",
				"120:a:1",
				"180:a:1",
				"240:a:1",
				"300:a:1",
				"360:a:1",
				"420:a:1",
			},
		}
		for expectedErr, dims := range testData {
			helper(expectedErr, dims)
		}
	})

	Convey("validate builder recipe", t, func() {
		vctx := &validation.Context{
			Context: context.Background(),
		}

		Convey("ok", func() {
			recipe := &pb.BuilderConfig_Recipe{
				Name:        "foo",
				CipdPackage: "infra/recipe_bundle",
				CipdVersion: "refs/heads/main",
				Properties:  []string{"a:b"},
				PropertiesJ: []string{"x:null", "y:true", "z:{\"zz\":true}"},
			}
			validateBuilderRecipe(vctx, recipe)
			So(vctx.Finalize(), ShouldBeNil)
		})

		Convey("bad", func() {
			recipe := &pb.BuilderConfig_Recipe{
				Properties:  []string{"", ":", "buildbucket:foobar", "x:y"},
				PropertiesJ: []string{"x:'y'", "y:b", "z"},
			}
			validateBuilderRecipe(vctx, recipe)
			ve := vctx.Finalize().(*validation.Error)
			So(len(ve.Errors), ShouldEqual, 8)
			So(ve.Errors[0].Error(), ShouldContainSubstring, "name: unspecified")
			So(ve.Errors[1].Error(), ShouldContainSubstring, "cipd_package: unspecified")
			So(ve.Errors[2].Error(), ShouldContainSubstring, "(properties #0 - ): doesn't have a colon")
			So(ve.Errors[3].Error(), ShouldContainSubstring, "(properties #1 - :): key not specified")
			So(ve.Errors[4].Error(), ShouldContainSubstring, "(properties #2 - buildbucket:foobar): reserved property")
			So(ve.Errors[5].Error(), ShouldContainSubstring, "(properties_j #0 - x:'y'): duplicate property")
			So(ve.Errors[6].Error(), ShouldContainSubstring, "(properties_j #1 - y:b): not a JSON object")
			So(ve.Errors[7].Error(), ShouldContainSubstring, "(properties_j #2 - z): doesn't have a colon")
		})

		Convey("bad $recipe_engine/runtime props", func() {
			runtime := `$recipe_engine/runtime:{"is_luci": false,"is_experimental": true, "unrecognized_is_fine": 1}`
			recipe := &pb.BuilderConfig_Recipe{
				Name:        "foo",
				CipdPackage: "infra/recipe_bundle",
				CipdVersion: "refs/heads/main",
				PropertiesJ: []string{runtime},
			}
			validateBuilderRecipe(vctx, recipe)
			ve, ok := vctx.Finalize().(*validation.Error)
			So(ok, ShouldEqual, true)
			So(len(ve.Errors), ShouldEqual, 2)
			allErrs := fmt.Sprintf("%s\n%s", ve.Errors[0].Error(), ve.Errors[1].Error())
			So(allErrs, ShouldContainSubstring, `key "is_luci": reserved key`)
			So(allErrs, ShouldContainSubstring, `key "is_experimental": reserved key`)
		})
	})
}

func TestUpdateProject(t *testing.T) {
	t.Parallel()

	// Strips the Proto field from each of the given *model.Bucket, returning a
	// slice whose ith index is the stripped *pb.Bucket value.
	// Needed because model.Bucket.Proto can only be compared with ShouldResembleProto
	// while model.Bucket can only be compared with ShouldResemble.
	stripBucketProtos := func(buckets []*model.Bucket) []*pb.Bucket {
		ret := make([]*pb.Bucket, len(buckets))
		for i, bkt := range buckets {
			if bkt == nil {
				ret[i] = nil
			} else {
				ret[i] = bkt.Proto
				bkt.Proto = nil
			}
		}
		return ret
	}
	stripBuilderProtos := func(buckets []*model.Builder) []*pb.BuilderConfig {
		ret := make([]*pb.BuilderConfig, len(buckets))
		for i, bldr := range buckets {
			if bldr == nil {
				ret[i] = nil
			} else {
				ret[i] = bldr.Config
				bldr.Config = nil
			}
		}
		return ret
	}

	Convey("update", t, func() {
		ctx := memory.UseWithAppID(context.Background(), "fake-cr-buildbucket")
		ctx = cfgclient.Use(ctx, &fakeCfgClient{})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx = txndefer.FilterRDS(ctx)
		origChromiumCfg, origChromiumRev, origDartCfg, origDartRev, origV8Cfg, origV8Rev :=
			chromiumBuildbucketCfg, chromiumRevision, dartBuildbucketCfg, dartRevision, v8BuildbucketCfg, v8Revision
		restoreCfgVars := func() {
			chromiumBuildbucketCfg, chromiumRevision = origChromiumCfg, origChromiumRev
			dartBuildbucketCfg, dartRevision = origDartCfg, origDartRev
			v8BuildbucketCfg, v8Revision = origV8Cfg, origV8Rev
		}

		// Datastore is empty. Mimic the first time receiving configs and store all
		// of them into Datastore.
		So(UpdateProjectCfg(ctx), ShouldBeNil)
		var actualBkts []*model.Bucket
		So(datastore.GetAll(ctx, datastore.NewQuery(model.BucketKind), &actualBkts), ShouldBeNil)
		So(len(actualBkts), ShouldEqual, 5)
		So(stripBucketProtos(actualBkts), ShouldResembleProto, []*pb.Bucket{
			{
				Name: "master.tryserver.chromium.linux",
			},
			{
				Name: "master.tryserver.chromium.win",
			},
			{
				Name: "try",
				Swarming: &pb.Swarming{
					Builders:                     []*pb.BuilderConfig{},
					TaskTemplateCanaryPercentage: &wrapperspb.UInt32Value{Value: uint32(10)},
				},
			},
			{
				Name: "try",
				Swarming: &pb.Swarming{
					Builders: []*pb.BuilderConfig{},
				},
			},
			{
				Name: "master.tryserver.v8",
			},
		})

		So(actualBkts, ShouldResemble, []*model.Bucket{
			{
				ID:       "master.tryserver.chromium.linux",
				Parent:   model.ProjectKey(ctx, "chromium"),
				Bucket:   "master.tryserver.chromium.linux",
				Schema:   CurrentBucketSchemaVersion,
				Revision: "deadbeef",
			},
			{
				ID:       "master.tryserver.chromium.win",
				Parent:   model.ProjectKey(ctx, "chromium"),
				Bucket:   "master.tryserver.chromium.win",
				Schema:   CurrentBucketSchemaVersion,
				Revision: "deadbeef",
			},
			{
				ID:       "try",
				Parent:   model.ProjectKey(ctx, "chromium"),
				Bucket:   "try",
				Schema:   CurrentBucketSchemaVersion,
				Revision: "deadbeef",
			},
			{
				ID:       "try",
				Parent:   model.ProjectKey(ctx, "dart"),
				Bucket:   "try",
				Schema:   CurrentBucketSchemaVersion,
				Revision: "deadbeef",
			},
			{
				ID:       "master.tryserver.v8",
				Parent:   model.ProjectKey(ctx, "v8"),
				Bucket:   "master.tryserver.v8",
				Schema:   CurrentBucketSchemaVersion,
				Revision: "sha1:502558141dd8e90ed88de7f1bf3fa430d4128966",
			},
		})

		var actualBuilders []*model.Builder
		So(datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind), &actualBuilders), ShouldBeNil)
		So(len(actualBuilders), ShouldEqual, 2)
		expectedBuilder1 := &pb.BuilderConfig{
			Name:                         "linux",
			Dimensions:                   []string{"os:Linux", "pool:luci.chromium.try"},
			SwarmingHost:                 "swarming.example.com",
			TaskTemplateCanaryPercentage: &wrapperspb.UInt32Value{Value: uint32(10)},
			Exe: &pb.Executable{
				CipdPackage: "infra/recipe_bundle",
				CipdVersion: "refs/heads/main",
				Cmd:         []string{"luciexe"},
			},
		}
		expectedBuilder2 := &pb.BuilderConfig{
			Name:       "linux",
			Dimensions: []string{"pool:Dart.LUCI"},
			Exe: &pb.Executable{
				CipdPackage: "infra/recipe_bundle",
				CipdVersion: "refs/heads/main",
				Cmd:         []string{"luciexe"},
			},
		}
		expectedBldrHash1, _, _ := computeBuilderHash(expectedBuilder1)
		expectedBldrHash2, _, _ := computeBuilderHash(expectedBuilder2)
		So(stripBuilderProtos(actualBuilders), ShouldResembleProto, []*pb.BuilderConfig{expectedBuilder1, expectedBuilder2})
		So(actualBuilders, ShouldResemble, []*model.Builder{
			{
				ID:         "linux",
				Parent:     model.BucketKey(ctx, "chromium", "try"),
				ConfigHash: expectedBldrHash1,
			},
			{
				ID:         "linux",
				Parent:     model.BucketKey(ctx, "dart", "try"),
				ConfigHash: expectedBldrHash2,
			},
		})

		Convey("with existing", func() {
			defer restoreCfgVars()

			// Add master.tryserver.chromium.mac
			// Update luci.chromium.try
			// Delete master.tryserver.chromium.win
			chromiumBuildbucketCfg = `
        buckets {
          name: "master.tryserver.chromium.linux"
        }
        buckets {
          name: "master.tryserver.chromium.mac"
        }
        buckets {
          name: "try"
          swarming {
            task_template_canary_percentage { value: 10 }
            builders {
              name: "linux"
              swarming_host: "swarming.updated.example.com"
              task_template_canary_percentage { value: 10 }
              dimensions: "os:Linux"
							exe {
								cipd_version: "refs/heads/main"
								cipd_package: "infra/recipe_bundle"
								cmd: ["luciexe"]
							}
            }
          }
        }
			`
			chromiumRevision = "new!"
			// Delete the entire v8 cfg
			v8BuildbucketCfg = ""

			So(UpdateProjectCfg(ctx), ShouldBeNil)
			var actualBkts []*model.Bucket
			So(datastore.GetAll(ctx, datastore.NewQuery(model.BucketKind), &actualBkts), ShouldBeNil)
			So(len(actualBkts), ShouldEqual, 4)
			So(stripBucketProtos(actualBkts), ShouldResembleProto, []*pb.Bucket{
				{
					Name: "master.tryserver.chromium.linux",
				},
				{
					Name: "master.tryserver.chromium.mac",
				},
				{
					Name: "try",
					Swarming: &pb.Swarming{
						Builders:                     []*pb.BuilderConfig{},
						TaskTemplateCanaryPercentage: &wrapperspb.UInt32Value{Value: uint32(10)},
					},
				},
				{
					Name: "try",
					Swarming: &pb.Swarming{
						Builders: []*pb.BuilderConfig{},
					},
				},
			})
			So(actualBkts, ShouldResemble, []*model.Bucket{
				{
					ID:       "master.tryserver.chromium.linux",
					Parent:   model.ProjectKey(ctx, "chromium"),
					Bucket:   "master.tryserver.chromium.linux",
					Schema:   CurrentBucketSchemaVersion,
					Revision: "new!",
				},
				{
					ID:       "master.tryserver.chromium.mac",
					Parent:   model.ProjectKey(ctx, "chromium"),
					Bucket:   "master.tryserver.chromium.mac",
					Schema:   CurrentBucketSchemaVersion,
					Revision: "new!",
				},
				{
					ID:       "try",
					Parent:   model.ProjectKey(ctx, "chromium"),
					Bucket:   "try",
					Schema:   CurrentBucketSchemaVersion,
					Revision: "new!",
				},
				{
					ID:       "try",
					Parent:   model.ProjectKey(ctx, "dart"),
					Bucket:   "try",
					Schema:   CurrentBucketSchemaVersion,
					Revision: "deadbeef",
				},
			})

			var actualBuilders []*model.Builder
			So(datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind), &actualBuilders), ShouldBeNil)
			So(len(actualBuilders), ShouldEqual, 2)
			expectedBuilder1 := &pb.BuilderConfig{
				Name:                         "linux",
				Dimensions:                   []string{"os:Linux", "pool:luci.chromium.try"},
				SwarmingHost:                 "swarming.updated.example.com",
				TaskTemplateCanaryPercentage: &wrapperspb.UInt32Value{Value: uint32(10)},
				Exe: &pb.Executable{
					CipdPackage: "infra/recipe_bundle",
					CipdVersion: "refs/heads/main",
					Cmd:         []string{"luciexe"},
				},
			}
			expectedBuilder2 := &pb.BuilderConfig{
				Name:       "linux",
				Dimensions: []string{"pool:Dart.LUCI"},
				Exe: &pb.Executable{
					CipdPackage: "infra/recipe_bundle",
					CipdVersion: "refs/heads/main",
					Cmd:         []string{"luciexe"},
				},
			}
			expectedBldrHash1, _, _ := computeBuilderHash(expectedBuilder1)
			expectedBldrHash2, _, _ := computeBuilderHash(expectedBuilder2)
			So(stripBuilderProtos(actualBuilders), ShouldResembleProto, []*pb.BuilderConfig{expectedBuilder1, expectedBuilder2})
			So(actualBuilders, ShouldResemble, []*model.Builder{
				{
					ID:         "linux",
					Parent:     model.BucketKey(ctx, "chromium", "try"),
					ConfigHash: expectedBldrHash1,
				},
				{
					ID:         "linux",
					Parent:     model.BucketKey(ctx, "dart", "try"),
					ConfigHash: expectedBldrHash2,
				},
			})
		})

		Convey("with broken configs", func() {
			defer restoreCfgVars()

			// Delete chromium and v8 configs
			chromiumBuildbucketCfg = ""
			v8BuildbucketCfg = ""

			dartBuildbucketCfg = "broken bucket cfg"
			dartBuildbucketCfg = "new!"

			So(UpdateProjectCfg(ctx), ShouldBeNil)

			// We must not delete buckets or builders defined in a project that
			// currently have a broken config.
			var actualBkts []*model.Bucket
			So(datastore.GetAll(ctx, datastore.NewQuery(model.BucketKind), &actualBkts), ShouldBeNil)
			So(len(actualBkts), ShouldEqual, 1)
			So(stripBucketProtos(actualBkts), ShouldResembleProto, []*pb.Bucket{
				{
					Name: "try",
					Swarming: &pb.Swarming{
						Builders: []*pb.BuilderConfig{},
					},
				},
			})
			So(actualBkts, ShouldResemble, []*model.Bucket{
				{
					ID:       "try",
					Parent:   model.ProjectKey(ctx, "dart"),
					Bucket:   "try",
					Schema:   CurrentBucketSchemaVersion,
					Revision: "deadbeef",
				},
			})

			var actualBuilders []*model.Builder
			So(datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind), &actualBuilders), ShouldBeNil)
			So(len(actualBuilders), ShouldEqual, 1)
			dartBuilder := &pb.BuilderConfig{
				Name:       "linux",
				Dimensions: []string{"pool:Dart.LUCI"},
				Exe: &pb.Executable{
					CipdPackage: "infra/recipe_bundle",
					CipdVersion: "refs/heads/main",
					Cmd:         []string{"luciexe"},
				},
			}
			dartBuilderHash, _, _ := computeBuilderHash(dartBuilder)
			So(stripBuilderProtos(actualBuilders), ShouldResembleProto, []*pb.BuilderConfig{dartBuilder})
			So(actualBuilders, ShouldResemble, []*model.Builder{
				{
					ID:         "linux",
					Parent:     model.BucketKey(ctx, "dart", "try"),
					ConfigHash: dartBuilderHash,
				},
			})
		})

		Convey("dart config return error", func() {
			defer restoreCfgVars()

			// Delete chromium and v8 configs
			chromiumBuildbucketCfg = ""
			v8BuildbucketCfg = ""

			// luci-config return server error when fetching dart config.
			dartBuildbucketCfg = "error"

			So(UpdateProjectCfg(ctx), ShouldBeNil)

			// Don't delete the stored buckets and builders when luci-config returns
			// an error for fetching that project config.
			var actualBkts []*model.Bucket
			So(datastore.GetAll(ctx, datastore.NewQuery(model.BucketKind), &actualBkts), ShouldBeNil)
			So(len(actualBkts), ShouldEqual, 1)
			So(stripBucketProtos(actualBkts), ShouldResembleProto, []*pb.Bucket{
				{
					Name: "try",
					Swarming: &pb.Swarming{
						Builders: []*pb.BuilderConfig{},
					},
				},
			})
			So(actualBkts, ShouldResemble, []*model.Bucket{
				{
					ID:       "try",
					Parent:   model.ProjectKey(ctx, "dart"),
					Bucket:   "try",
					Schema:   CurrentBucketSchemaVersion,
					Revision: "deadbeef",
				},
			})

			var actualBuilders []*model.Builder
			So(datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind), &actualBuilders), ShouldBeNil)
			So(len(actualBuilders), ShouldEqual, 1)
			dartBuilder := &pb.BuilderConfig{
				Name:       "linux",
				Dimensions: []string{"pool:Dart.LUCI"},
				Exe: &pb.Executable{
					CipdPackage: "infra/recipe_bundle",
					CipdVersion: "refs/heads/main",
					Cmd:         []string{"luciexe"},
				},
			}
			dartBuilderHash, _, _ := computeBuilderHash(dartBuilder)
			So(stripBuilderProtos(actualBuilders), ShouldResembleProto, []*pb.BuilderConfig{dartBuilder})
			So(actualBuilders, ShouldResemble, []*model.Builder{
				{
					ID:         "linux",
					Parent:     model.BucketKey(ctx, "dart", "try"),
					ConfigHash: dartBuilderHash,
				},
			})
		})

		Convey("large builders count", func() {
			// clear dart configs first
			defer restoreCfgVars()
			dartBuildbucketCfg = `buckets {name: "try"}`
			dartRevision = `clear_dart`

			So(UpdateProjectCfg(ctx), ShouldBeNil)
			actualBucket := &model.Bucket{ID: "try", Parent: model.ProjectKey(ctx, "dart")}
			So(datastore.Get(ctx, actualBucket), ShouldBeNil)
			So(actualBucket.Revision, ShouldEqual, "clear_dart")
			var actualBuilders []*model.Builder
			So(datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind).Ancestor(model.BucketKey(ctx, "dart", "try")).Order("__key__"), &actualBuilders), ShouldBeNil)
			So(len(actualBuilders), ShouldEqual, 0)

			Convey("to put 499 builders", func() {
				defer restoreCfgVars()

				bldrsCfg := ""
				for i := 0; i < 499; i++ {
					bldrsCfg += fmt.Sprintf("builders {name: \"builder%d\"}\n", i)
				}
				dartBuildbucketCfg = fmt.Sprintf(`buckets {name: "try"swarming {%s}}`, bldrsCfg)
				dartRevision = "put499"

				So(UpdateProjectCfg(ctx), ShouldBeNil)

				actualBucket := &model.Bucket{ID: "try", Parent: model.ProjectKey(ctx, "dart")}
				So(datastore.Get(ctx, actualBucket), ShouldBeNil)
				So(actualBucket.Revision, ShouldEqual, "put499")
				var actualBuilders []*model.Builder
				So(datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind).Ancestor(model.BucketKey(ctx, "dart", "try")).Order("__key__"), &actualBuilders), ShouldBeNil)
				So(len(actualBuilders), ShouldEqual, 499)
			})

			Convey("to put 500 builders", func() {
				defer restoreCfgVars()

				bldrsCfg := ""
				for i := 0; i < 500; i++ {
					bldrsCfg += fmt.Sprintf("builders {name: \"builder%d\"}\n", i)
				}
				dartBuildbucketCfg = fmt.Sprintf(`buckets {name: "try"swarming {%s}}`, bldrsCfg)
				dartRevision = "put500"

				So(UpdateProjectCfg(ctx), ShouldBeNil)

				actualBucket := &model.Bucket{ID: "try", Parent: model.ProjectKey(ctx, "dart")}
				So(datastore.Get(ctx, actualBucket), ShouldBeNil)
				So(actualBucket.Revision, ShouldEqual, "put500")
				var actualBuilders []*model.Builder
				So(datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind).Ancestor(model.BucketKey(ctx, "dart", "try")).Order("__key__"), &actualBuilders), ShouldBeNil)
				So(len(actualBuilders), ShouldEqual, 500)
			})

			Convey("to put 1105 builders", func() {
				defer restoreCfgVars()

				bldrsCfg := ""
				for i := 0; i < 1105; i++ {
					bldrsCfg += fmt.Sprintf("builders {name: \"builder%d\"}\n", i)
				}
				dartBuildbucketCfg = fmt.Sprintf(`buckets {name: "try"swarming {%s}}`, bldrsCfg)
				dartRevision = "put1105"

				So(UpdateProjectCfg(ctx), ShouldBeNil)

				actualBucket := &model.Bucket{ID: "try", Parent: model.ProjectKey(ctx, "dart")}
				So(datastore.Get(ctx, actualBucket), ShouldBeNil)
				So(actualBucket.Revision, ShouldEqual, "put1105")
				var actualBuilders []*model.Builder
				So(datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind).Ancestor(model.BucketKey(ctx, "dart", "try")).Order("__key__"), &actualBuilders), ShouldBeNil)
				So(len(actualBuilders), ShouldEqual, 1105)

				Convey("delete 111 and update 994", func() {
					bldrsCfg := ""
					for i := 0; i < 1105; i++ {
						// delete builders which the name ends with "1".
						if i%10 == 1 {
							continue
						}
						bldrsCfg += fmt.Sprintf("builders {name: \"builder%d\" \n dimensions: \"pool:newly_added\"}\n", i)
					}
					dartBuildbucketCfg = fmt.Sprintf(`buckets {name: "try"swarming {%s}}`, bldrsCfg)
					dartRevision = "del111_update994"

					So(UpdateProjectCfg(ctx), ShouldBeNil)

					actualBucket := &model.Bucket{ID: "try", Parent: model.ProjectKey(ctx, "dart")}
					So(datastore.Get(ctx, actualBucket), ShouldBeNil)
					So(actualBucket.Revision, ShouldEqual, "del111_update994")
					var actualBuilders []*model.Builder
					So(datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind).Ancestor(model.BucketKey(ctx, "dart", "try")).Order("__key__"), &actualBuilders), ShouldBeNil)
					So(len(actualBuilders), ShouldEqual, 994)
					for _, bldr := range actualBuilders {
						So(strings.HasSuffix(bldr.ID, "1"), ShouldBeFalse)
						So(bldr.Config.Dimensions[0], ShouldEqual, "pool:newly_added")
					}
				})

				Convey("delete 994 and update 111", func() {
					bldrsCfg := ""
					for i := 0; i < 1105; i++ {
						// only keep builders which the name ends with "1" and update them.
						if i%10 == 1 {
							bldrsCfg += fmt.Sprintf("builders {name: \"builder%d\" \n dimensions: \"pool:newly_added\"}\n", i)
						}
					}
					dartBuildbucketCfg = fmt.Sprintf(`buckets {name: "try"swarming {%s}}`, bldrsCfg)
					dartRevision = "del994_update111"

					So(UpdateProjectCfg(ctx), ShouldBeNil)

					actualBucket := &model.Bucket{ID: "try", Parent: model.ProjectKey(ctx, "dart")}
					So(datastore.Get(ctx, actualBucket), ShouldBeNil)
					So(actualBucket.Revision, ShouldEqual, "del994_update111")
					var actualBuilders []*model.Builder
					So(datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind).Ancestor(model.BucketKey(ctx, "dart", "try")).Order("__key__"), &actualBuilders), ShouldBeNil)
					So(len(actualBuilders), ShouldEqual, 111)
					for _, bldr := range actualBuilders {
						So(strings.HasSuffix(bldr.ID, "1"), ShouldBeTrue)
						So(bldr.Config.Dimensions[0], ShouldEqual, "pool:newly_added")
					}
				})
			})
		})

		Convey("large builder content", func() {
			// clear dart configs first
			defer restoreCfgVars()
			dartBuildbucketCfg = `buckets {name: "try"}`
			dartRevision = `clear_dart`

			So(UpdateProjectCfg(ctx), ShouldBeNil)
			actualBucket := &model.Bucket{ID: "try", Parent: model.ProjectKey(ctx, "dart")}
			So(datastore.Get(ctx, actualBucket), ShouldBeNil)
			So(actualBucket.Revision, ShouldEqual, "clear_dart")
			var actualBuilders []*model.Builder
			So(datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind).Ancestor(model.BucketKey(ctx, "dart", "try")).Order("__key__"), &actualBuilders), ShouldBeNil)
			So(len(actualBuilders), ShouldEqual, 0)

			originalMaxBatchSize := maxBatchSize
			defer func() {
				maxBatchSize = originalMaxBatchSize
			}()
			maxBatchSize = 200

			Convey("a single too large", func() {
				defer restoreCfgVars()

				large := ""
				for i := 0; i < 30; i++ {
					large += "0123456789"
				}
				dartBuildbucketCfg = fmt.Sprintf(`buckets {name: "try" swarming {builders {name: "%s"}}}`, large)
				dartRevision = "one_large"

				err := UpdateProjectCfg(ctx)
				So(err, ShouldErrLike, "size exceeds 200 bytes")
			})

			Convey("the sum > maxBatchSize while builders count < 500", func() {
				defer restoreCfgVars()

				bldrsCfg := ""
				for i := 0; i < 212; i++ {
					bldrsCfg += fmt.Sprintf("builders {name: \"medium_size_builder_%d\"}\n", i)
				}

				dartBuildbucketCfg = fmt.Sprintf(`buckets {name: "try"swarming {%s}}`, bldrsCfg)
				dartRevision = "sum_large"

				So(UpdateProjectCfg(ctx), ShouldBeNil)

				actualBucket := &model.Bucket{ID: "try", Parent: model.ProjectKey(ctx, "dart")}
				So(datastore.Get(ctx, actualBucket), ShouldBeNil)
				So(actualBucket.Revision, ShouldEqual, "sum_large")
				var actualBuilders []*model.Builder
				So(datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind).Ancestor(model.BucketKey(ctx, "dart", "try")).Order("__key__"), &actualBuilders), ShouldBeNil)
				So(len(actualBuilders), ShouldEqual, 212)
			})
		})
	})
}
