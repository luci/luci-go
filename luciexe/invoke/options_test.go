// Copyright 2019 The LUCI Authors.
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

package invoke

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/luciexe"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

var tempEnvVar string

func init() {
	if runtime.GOOS == "windows" {
		tempEnvVar = "TMP"
	} else {
		tempEnvVar = "TMPDIR"
	}
}

var nullLogdogEnv = environ.New([]string{
	bootstrap.EnvStreamServerPath + "=null",
	bootstrap.EnvStreamProject + "=testing",
	bootstrap.EnvStreamPrefix + "=prefix",
	bootstrap.EnvCoordinatorHost + "=test.example.com",
})

func commonOptions() (ctx context.Context, o *Options, tdir string, closer func()) {
	ctx, _ = testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
	ctx = lucictx.SetDeadline(ctx, nil)

	// luciexe protocol requires the 'host' application to manage the tempdir.
	// In this context the test binary is the host. It's more
	// convenient+accurate to have a non-hermetic test than to mock this out.
	oldTemp := os.Getenv(tempEnvVar)
	closer = func() {
		if err := os.Setenv(tempEnvVar, oldTemp); err != nil {
			panic(err)
		}
		if tdir != "" {
			So(os.RemoveAll(tdir), ShouldBeNil)
		}
	}

	var err error
	if tdir, err = ioutil.TempDir("", "luciexe_test"); err != nil {
		closer() // want to do cleanup if ioutil.TempDir failed
		So(err, ShouldBeNil)
	}
	if err := os.Setenv(tempEnvVar, tdir); err != nil {
		panic(err)
	}

	o = &Options{
		Env: nullLogdogEnv.Clone(),
	}

	return
}

func TestOptionsGeneral(t *testing.T) {
	Convey(`test Options (general)`, t, func() {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)

		Convey(`works with nil`, func() {
			// TODO(iannucci): really gotta put all these envvars in LUCI_CONTEXT...
			oldVals := map[string]string{}
			for k, v := range nullLogdogEnv.Map() {
				oldVals[k] = os.Getenv(k)
				if err := os.Setenv(k, v); err != nil {
					panic(err)
				}
			}
			defer func() {
				for k, v := range oldVals {
					if err := os.Setenv(k, v); err != nil {
						panic(err)
					}
				}
			}()
			expected := stringset.NewFromSlice(luciexe.TempDirEnvVars...)
			for key := range environ.System().Map() {
				expected.Add(key)
			}
			expected.Add(lucictx.EnvKey)

			lo, _, err := ((*Options)(nil)).rationalize(ctx)
			So(err, ShouldBeNil)

			envKeys := stringset.New(expected.Len())
			lo.env.Iter(func(k, _ string) error {
				envKeys.Add(k)
				return nil
			})
			So(envKeys, ShouldResemble, expected)
		})
	})
}

func TestOptionsNamespace(t *testing.T) {
	Convey(`test Options.Namespace`, t, func() {
		ctx, o, _, closer := commonOptions()
		defer closer()

		nowP := timestamppb.New(clock.Now(ctx))

		Convey(`default`, func() {
			lo, _, err := o.rationalize(ctx)
			So(err, ShouldBeNil)
			So(lo.env.Get(bootstrap.EnvNamespace), ShouldResemble, "")
			So(lo.step, ShouldBeNil)
		})

		Convey(`errors`, func() {
			Convey(`bad clock`, func() {
				o.Namespace = "yarp"
				ctx, _ := testclock.UseTime(ctx, time.Unix(-100000000000, 0))

				_, _, err := o.rationalize(ctx)
				So(err, ShouldErrLike, "preparing namespace: invalid StartTime")
			})
		})

		Convey(`toplevel`, func() {
			o.Namespace = "u"
			lo, _, err := o.rationalize(ctx)
			So(err, ShouldBeNil)
			So(lo.env.Get(bootstrap.EnvNamespace), ShouldResemble, "u")
			So(lo.step, ShouldResemble, &bbpb.Step{
				Name:      "u",
				StartTime: nowP,
				Status:    bbpb.Status_STARTED,
				Logs: []*bbpb.Log{
					{Name: "$build.proto", Url: "u/build.proto"},
					{Name: "stdout", Url: "u/stdout"},
					{Name: "stderr", Url: "u/stderr"},
				},
			})
			So(lo.step.Logs[0].Url, ShouldEqual, "u/build.proto")
		})

		Convey(`nested`, func() {
			o.Env.Set(bootstrap.EnvNamespace, "u/bar")
			o.Namespace = "sub"
			lo, _, err := o.rationalize(ctx)
			So(err, ShouldBeNil)
			So(lo.env.Get(bootstrap.EnvNamespace), ShouldResemble, "u/bar/sub")
			So(lo.step, ShouldResemble, &bbpb.Step{
				Name:      "sub", // host application will swizzle this
				StartTime: nowP,
				Status:    bbpb.Status_STARTED,
				Logs: []*bbpb.Log{
					{Name: "$build.proto", Url: "sub/build.proto"},
					{Name: "stdout", Url: "sub/stdout"},
					{Name: "stderr", Url: "sub/stderr"},
				},
			})
			So(lo.step.Logs[0].Url, ShouldEqual, "sub/build.proto")
		})

		Convey(`deeply nested`, func() {
			o.Env.Set(bootstrap.EnvNamespace, "u")
			o.Namespace = "step|!!cool!!|sub"
			lo, _, err := o.rationalize(ctx)
			So(err, ShouldBeNil)
			So(lo.env.Get(bootstrap.EnvNamespace),
				ShouldResemble, "u/step/s___cool__/sub")
			So(lo.step, ShouldResemble, &bbpb.Step{
				Name:      "step|!!cool!!|sub", // host application will swizzle this
				StartTime: nowP,
				Status:    bbpb.Status_STARTED,
				Logs: []*bbpb.Log{
					{Name: "$build.proto", Url: "step/s___cool__/sub/build.proto"},
					{Name: "stdout", Url: "step/s___cool__/sub/stdout"},
					{Name: "stderr", Url: "step/s___cool__/sub/stderr"},
				},
			})
			So(lo.step.Logs[0].Url, ShouldEqual, "step/s___cool__/sub/build.proto")
		})
	})
}

func TestOptionsCacheDir(t *testing.T) {
	Convey(`Options.CacheDir`, t, func() {
		ctx, o, tdir, closer := commonOptions()
		defer closer()

		Convey(`default`, func() {
			_, ctx, err := o.rationalize(ctx)
			So(err, ShouldBeNil)
			lexe := lucictx.GetLUCIExe(ctx)
			So(lexe, ShouldNotBeNil)
			So(lexe.CacheDir, ShouldStartWith, tdir)
		})

		Convey(`override`, func() {
			o.CacheDir = filepath.Join(tdir, "cache")
			So(os.Mkdir(o.CacheDir, 0777), ShouldBeNil)
			_, ctx, err := o.rationalize(ctx)
			So(err, ShouldBeNil)
			lexe := lucictx.GetLUCIExe(ctx)
			So(lexe, ShouldNotBeNil)
			So(lexe.CacheDir, ShouldEqual, o.CacheDir)
		})

		Convey(`errors`, func() {
			Convey(`empty cache dir set`, func() {
				ctx := lucictx.SetLUCIExe(ctx, &lucictx.LUCIExe{})
				_, _, err := o.rationalize(ctx)
				So(err, ShouldErrLike, `"cache_dir" is empty`)
			})

			Convey(`bad override (doesn't exist)`, func() {
				o.CacheDir = filepath.Join(tdir, "cache")
				_, _, err := o.rationalize(ctx)
				So(err, ShouldErrLike, "checking CacheDir: dir does not exist")
			})

			Convey(`bad override (not a dir)`, func() {
				o.CacheDir = filepath.Join(tdir, "cache")
				So(ioutil.WriteFile(o.CacheDir, []byte("not a dir"), 0666), ShouldBeNil)
				_, _, err := o.rationalize(ctx)
				So(err, ShouldErrLike, "checking CacheDir: path is not a directory")
			})
		})
	})
}

func TestOptionsCollectOutput(t *testing.T) {
	Convey(`Options.CollectOutput`, t, func() {
		ctx, o, tdir, closer := commonOptions()
		defer closer()

		Convey(`default`, func() {
			lo, _, err := o.rationalize(ctx)
			So(err, ShouldBeNil)
			So(lo.args, ShouldBeEmpty)
			out, err := luciexe.ReadBuildFile(lo.collectPath)
			So(err, ShouldBeNil)
			So(out, ShouldBeNil)
		})

		Convey(`errors`, func() {
			Convey(`bad extension`, func() {
				o.CollectOutputPath = filepath.Join(tdir, "output.fleem")
				_, _, err := o.rationalize(ctx)
				So(err, ShouldErrLike, "bad extension for build proto file path")
			})

			Convey(`already exists`, func() {
				outPath := filepath.Join(tdir, "output.pb")
				o.CollectOutputPath = outPath
				So(ioutil.WriteFile(outPath, nil, 0666), ShouldBeNil)
				_, _, err := o.rationalize(ctx)
				So(err, ShouldErrLike, "CollectOutputPath points to an existing file")
			})

			Convey(`parent is not a dir`, func() {
				parDir := filepath.Join(tdir, "parent")
				So(ioutil.WriteFile(parDir, nil, 0666), ShouldBeNil)
				o.CollectOutputPath = filepath.Join(parDir, "out.pb")

				_, _, err := o.rationalize(ctx)
				So(err, ShouldErrLike, "checking CollectOutputPath's parent: path is not a directory")
			})

			Convey(`no parent folder`, func() {
				o.CollectOutputPath = filepath.Join(tdir, "extra", "output.fleem")
				_, _, err := o.rationalize(ctx)
				So(err, ShouldErrLike, "checking CollectOutputPath's parent: dir does not exist")
			})
		})

		Convey(`parseOutput`, func() {
			expected := &bbpb.Build{SummaryMarkdown: "I'm a summary."}
			testParseOutput := func(expectedData []byte, checkFilename func(string)) {
				lo, _, err := o.rationalize(ctx)
				So(err, ShouldBeNil)
				So(lo.args, ShouldHaveLength, 2)
				So(lo.args[0], ShouldEqual, luciexe.OutputCLIArg)
				checkFilename(lo.args[1])

				_, err = luciexe.ReadBuildFile(lo.collectPath)
				So(err, ShouldErrLike, "opening build file")

				So(ioutil.WriteFile(lo.args[1], expectedData, 0666), ShouldBeNil)

				build, err := luciexe.ReadBuildFile(lo.collectPath)
				So(err, ShouldBeNil)
				So(build, ShouldResembleProto, expected)
			}

			Convey(`collect but no specific file`, func() {
				o.CollectOutput = true
				data, err := proto.Marshal(expected)
				So(err, ShouldBeNil)
				testParseOutput(data, func(filename string) {
					So(filename, ShouldStartWith, tdir)
					So(filename, ShouldEndWith, luciexe.BuildFileCodecBinary.FileExtension())
				})
			})

			Convey(`collect from a binary file`, func() {
				o.CollectOutput = true
				outPath := filepath.Join(tdir, "output.pb")
				o.CollectOutputPath = outPath

				data, err := proto.Marshal(expected)
				So(err, ShouldBeNil)

				testParseOutput(data, func(filename string) {
					So(filename, ShouldEqual, outPath)
				})
			})

			Convey(`collect from a json file`, func() {
				o.CollectOutput = true
				outPath := filepath.Join(tdir, "output.json")
				o.CollectOutputPath = outPath

				data, err := (&jsonpb.Marshaler{OrigName: true}).MarshalToString(expected)
				So(err, ShouldBeNil)

				testParseOutput([]byte(data), func(filename string) {
					So(filename, ShouldEqual, outPath)
				})
			})

			Convey(`collect from a textpb file`, func() {
				o.CollectOutput = true
				outPath := filepath.Join(tdir, "output.textpb")
				o.CollectOutputPath = outPath

				testParseOutput([]byte(expected.String()), func(filename string) {
					So(filename, ShouldEqual, outPath)
				})
			})
		})
	})
}

func TestOptionsEnv(t *testing.T) {
	Convey(`Env`, t, func() {
		ctx, o, _, closer := commonOptions()
		defer closer()

		Convey(`default`, func() {
			lo, _, err := o.rationalize(ctx)
			So(err, ShouldBeNil)

			expected := stringset.NewFromSlice(luciexe.TempDirEnvVars...)
			for key := range o.Env.Map() {
				expected.Add(key)
			}
			expected.Add(lucictx.EnvKey)

			actual := stringset.New(expected.Len())
			for key := range lo.env.Map() {
				actual.Add(key)
			}

			So(actual, ShouldResemble, expected)
		})
	})
}

func TestOptionsStdio(t *testing.T) {
	Convey(`stdio`, t, func() {
		ctx, o, _, closer := commonOptions()
		defer closer()

		Convey(`default`, func() {
			lo, _, err := o.rationalize(ctx)
			So(err, ShouldBeNil)
			So(lo.stderr, ShouldNotBeNil)
			So(lo.stdout, ShouldNotBeNil)
		})

		Convey(`errors`, func() {
			Convey(`bad bootstrap (missing)`, func() {
				o.Env.Remove(bootstrap.EnvStreamServerPath)
				_, _, err := o.rationalize(ctx)
				So(err, ShouldErrLike, "Logdog Butler environment required")
			})

			Convey(`bad bootstrap (malformed)`, func() {
				o.Env.Set(bootstrap.EnvStreamPrefix, "!!!!")
				_, _, err := o.rationalize(ctx)
				So(err, ShouldErrLike, `failed to validate prefix "!!!!"`)
			})
		})
	})
}

func TestOptionsExtraDirs(t *testing.T) {
	Convey(`tempDir+workDir`, t, func() {
		ctx, o, tdir, closer := commonOptions()
		defer closer()

		Convey(`provided BaseDir`, func() {
			o.BaseDir = filepath.Join(tdir, "base")
			So(os.Mkdir(o.BaseDir, 0777), ShouldBeNil)
			lo, _, err := o.rationalize(ctx)
			So(err, ShouldBeNil)
			So(lo.env.Get("TMP"), ShouldStartWith, o.BaseDir)
			So(lo.workDir, ShouldStartWith, o.BaseDir)
		})

		Convey(`provided BaseDir does not exist`, func() {
			o.BaseDir = filepath.Join(tdir, "base")
			_, _, err := o.rationalize(ctx)
			So(err, ShouldErrLike, "checking BaseDir: dir does not exist")
		})

		Convey(`provided BaseDir is not a directory`, func() {
			o.BaseDir = filepath.Join(tdir, "base")
			So(ioutil.WriteFile(o.BaseDir, []byte("not a dir"), 0666), ShouldBeNil)
			_, _, err := o.rationalize(ctx)
			So(err, ShouldErrLike, "checking BaseDir: path is not a directory")
		})

		Convey(`fallback to temp`, func() {
			lo, _, err := o.rationalize(ctx)
			So(err, ShouldBeNil)
			So(lo.env.Get("TMP"), ShouldStartWith, tdir)
			So(lo.workDir, ShouldStartWith, tdir)
		})
	})
}
