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

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func TestProject(t *testing.T) {
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
					acls {
						role: WRITER
						group: "writers"
					}
				}
				buckets {
					name: "good.name2"
					acls {
						role: READER
						identity: "a@a.com"
					}
					acls {
						role: READER
						identity: "user:b@a.com"
					}
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
				acl_sets { name: "a" }
				buckets {
					name: "a"
					acls {
						role: READER
						group: "writers"
						identity: "a@a.com"
					}
					acls {
						role: READER
					}
				}
				buckets {
					name: "a"
					acl_sets: "a"
					acls {
						role: READER
						identity: "ldap"
					}
					acls {
						role: READER
						group: ";%:"
					}
				}
				buckets {}
				buckets { name: "luci.x" }
			`
			So(validateProjectCfg(vctx, configSet, path, []byte(badCfg)), ShouldBeNil)
			ve, ok := vctx.Finalize().(*validation.Error)
			So(ok, ShouldEqual, true)
			So(len(ve.Errors), ShouldEqual, 9)
			So(ve.Errors[0].Error(), ShouldContainSubstring, "acl_sets: deprecated (use go/lucicfg)")
			So(ve.Errors[1].Error(), ShouldContainSubstring, "(buckets #0 - a / acls #0): either group or identity must be set, not both")
			So(ve.Errors[2].Error(), ShouldContainSubstring, "(buckets #0 - a / acls #1): group or identity must be set")
			So(ve.Errors[3].Error(), ShouldContainSubstring, "(buckets #1 - a): duplicate bucket name \"a\"")
			So(ve.Errors[4].Error(), ShouldContainSubstring, "(buckets #1 - a / acls #0): \"ldap\" invalid: auth: bad value \"ldap\" for identity kind \"user\"")
			So(ve.Errors[5].Error(), ShouldContainSubstring, "(buckets #1 - a / acls #1): invalid group: ;%:")
			So(ve.Errors[6].Error(), ShouldContainSubstring, "(buckets #1 - a): acl_sets: deprecated (use go/lucicfg)")
			So(ve.Errors[7].Error(), ShouldContainSubstring, "(buckets #2 - ): invalid name \"\": bucket name is not specified")
			So(ve.Errors[8].Error(), ShouldContainSubstring, "(buckets #3 - luci.x): invalid name \"luci.x\": must start with 'luci.test.' because it starts with 'luci.' and is defined in the \"test\" project")
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
