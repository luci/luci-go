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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/lucictx"

	"go.chromium.org/luci/luciexe"
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

func commonOptions(t testing.TB) (ctx context.Context, o *Options, tdir string) {
	t.Helper()
	ctx, _ = testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
	ctx = lucictx.SetDeadline(ctx, nil)

	// luciexe protocol requires the 'host' application to manage the tempdir.
	// In this context the test binary is the host. It's more
	// convenient+accurate to have a non-hermetic test than to mock this out.
	oldTemp := os.Getenv(tempEnvVar)
	t.Cleanup(func() {
		if err := os.Setenv(tempEnvVar, oldTemp); err != nil {
			panic(err)
		}
	})

	tdir = t.TempDir()
	if err := os.Setenv(tempEnvVar, tdir); err != nil {
		panic(err)
	}

	o = &Options{
		Env: nullLogdogEnv.Clone(),
	}

	return
}

func TestOptionsGeneral(t *testing.T) {
	ftt.Run(`test Options (general)`, t, func(t *ftt.Test) {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)

		t.Run(`works with nil`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)

			envKeys := stringset.New(expected.Len())
			lo.env.Iter(func(k, _ string) error {
				envKeys.Add(k)
				return nil
			})
			assert.Loosely(t, envKeys, should.Match(expected))
		})
	})
}

func TestOptionsNamespace(t *testing.T) {
	ftt.Run(`test Options.Namespace`, t, func(t *ftt.Test) {
		ctx, o, _ := commonOptions(t)

		nowP := timestamppb.New(clock.Now(ctx))

		t.Run(`default`, func(t *ftt.Test) {
			lo, _, err := o.rationalize(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, lo.env.Get(bootstrap.EnvNamespace), should.BeBlank)
			assert.Loosely(t, lo.step, should.BeNil)
		})

		t.Run(`errors`, func(t *ftt.Test) {
			t.Run(`bad clock`, func(t *ftt.Test) {
				o.Namespace = "yarp"
				ctx, _ := testclock.UseTime(ctx, time.Unix(-100000000000, 0))

				_, _, err := o.rationalize(ctx)
				assert.Loosely(t, err, should.ErrLike("preparing namespace: invalid StartTime"))
			})
		})

		t.Run(`toplevel`, func(t *ftt.Test) {
			o.Namespace = "u"
			lo, _, err := o.rationalize(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, lo.env.Get(bootstrap.EnvNamespace), should.Match("u"))
			assert.Loosely(t, lo.step, should.Match(&bbpb.Step{
				Name:      "u",
				StartTime: nowP,
				Status:    bbpb.Status_STARTED,
				Logs: []*bbpb.Log{
					{Name: "stdout", Url: "u/stdout"},
					{Name: "stderr", Url: "u/stderr"},
				},
				MergeBuild: &bbpb.Step_MergeBuild{
					FromLogdogStream: "u/build.proto",
				},
			}))
		})

		t.Run(`nested`, func(t *ftt.Test) {
			o.Env.Set(bootstrap.EnvNamespace, "u/bar")
			o.Namespace = "sub"
			lo, _, err := o.rationalize(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, lo.env.Get(bootstrap.EnvNamespace), should.Match("u/bar/sub"))
			assert.Loosely(t, lo.step, should.Match(&bbpb.Step{
				Name:      "sub", // host application will swizzle this
				StartTime: nowP,
				Status:    bbpb.Status_STARTED,
				Logs: []*bbpb.Log{
					{Name: "stdout", Url: "sub/stdout"},
					{Name: "stderr", Url: "sub/stderr"},
				},
				MergeBuild: &bbpb.Step_MergeBuild{
					FromLogdogStream: "sub/build.proto",
				},
			}))
		})

		t.Run(`deeply nested`, func(t *ftt.Test) {
			o.Env.Set(bootstrap.EnvNamespace, "u")
			o.Namespace = "step|!!cool!!|sub"
			lo, _, err := o.rationalize(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, lo.env.Get(bootstrap.EnvNamespace),
				should.Match("u/step/s___cool__/sub"))
			assert.Loosely(t, lo.step, should.Match(&bbpb.Step{
				Name:      "step|!!cool!!|sub", // host application will swizzle this
				StartTime: nowP,
				Status:    bbpb.Status_STARTED,
				Logs: []*bbpb.Log{
					{Name: "stdout", Url: "step/s___cool__/sub/stdout"},
					{Name: "stderr", Url: "step/s___cool__/sub/stderr"},
				},
				MergeBuild: &bbpb.Step_MergeBuild{
					FromLogdogStream: "step/s___cool__/sub/build.proto",
				},
			}))
		})
	})
}

func TestOptionsCacheDir(t *testing.T) {
	ftt.Run(`Options.CacheDir`, t, func(t *ftt.Test) {
		ctx, o, tdir := commonOptions(t)

		t.Run(`default`, func(t *ftt.Test) {
			_, ctx, err := o.rationalize(ctx)
			assert.Loosely(t, err, should.BeNil)
			lexe := lucictx.GetLUCIExe(ctx)
			assert.Loosely(t, lexe, should.NotBeNil)
			assert.Loosely(t, lexe.CacheDir, should.HavePrefix(tdir))
		})

		t.Run(`override`, func(t *ftt.Test) {
			o.CacheDir = filepath.Join(tdir, "cache")
			assert.Loosely(t, os.Mkdir(o.CacheDir, 0777), should.BeNil)
			_, ctx, err := o.rationalize(ctx)
			assert.Loosely(t, err, should.BeNil)
			lexe := lucictx.GetLUCIExe(ctx)
			assert.Loosely(t, lexe, should.NotBeNil)
			assert.Loosely(t, lexe.CacheDir, should.Equal(o.CacheDir))
		})

		t.Run(`errors`, func(t *ftt.Test) {
			t.Run(`empty cache dir set`, func(t *ftt.Test) {
				ctx := lucictx.SetLUCIExe(ctx, &lucictx.LUCIExe{})
				_, _, err := o.rationalize(ctx)
				assert.Loosely(t, err, should.ErrLike(`"cache_dir" is empty`))
			})

			t.Run(`bad override (doesn't exist)`, func(t *ftt.Test) {
				o.CacheDir = filepath.Join(tdir, "cache")
				_, _, err := o.rationalize(ctx)
				assert.Loosely(t, err, should.ErrLike("checking CacheDir: dir does not exist"))
			})

			t.Run(`bad override (not a dir)`, func(t *ftt.Test) {
				o.CacheDir = filepath.Join(tdir, "cache")
				assert.Loosely(t, os.WriteFile(o.CacheDir, []byte("not a dir"), 0666), should.BeNil)
				_, _, err := o.rationalize(ctx)
				assert.Loosely(t, err, should.ErrLike("checking CacheDir: path is not a directory"))
			})
		})
	})
}

func TestOptionsCollectOutput(t *testing.T) {
	ftt.Run(`Options.CollectOutput`, t, func(t *ftt.Test) {
		ctx, o, tdir := commonOptions(t)

		t.Run(`default`, func(t *ftt.Test) {
			lo, _, err := o.rationalize(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, lo.args, should.BeEmpty)
			out, err := luciexe.ReadBuildFile(lo.collectPath)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, out, should.BeNil)
		})

		t.Run(`errors`, func(t *ftt.Test) {
			t.Run(`bad extension`, func(t *ftt.Test) {
				o.CollectOutputPath = filepath.Join(tdir, "output.fleem")
				_, _, err := o.rationalize(ctx)
				assert.Loosely(t, err, should.ErrLike("bad extension for build proto file path"))
			})

			t.Run(`already exists`, func(t *ftt.Test) {
				outPath := filepath.Join(tdir, "output.pb")
				o.CollectOutputPath = outPath
				assert.Loosely(t, os.WriteFile(outPath, nil, 0666), should.BeNil)
				_, _, err := o.rationalize(ctx)
				assert.Loosely(t, err, should.ErrLike("CollectOutputPath points to an existing file"))
			})

			t.Run(`parent is not a dir`, func(t *ftt.Test) {
				parDir := filepath.Join(tdir, "parent")
				assert.Loosely(t, os.WriteFile(parDir, nil, 0666), should.BeNil)
				o.CollectOutputPath = filepath.Join(parDir, "out.pb")

				_, _, err := o.rationalize(ctx)
				assert.Loosely(t, err, should.ErrLike("checking CollectOutputPath's parent: path is not a directory"))
			})

			t.Run(`no parent folder`, func(t *ftt.Test) {
				o.CollectOutputPath = filepath.Join(tdir, "extra", "output.fleem")
				_, _, err := o.rationalize(ctx)
				assert.Loosely(t, err, should.ErrLike("checking CollectOutputPath's parent: dir does not exist"))
			})
		})

		t.Run(`parseOutput`, func(t *ftt.Test) {
			expected := &bbpb.Build{SummaryMarkdown: "I'm a summary."}
			testParseOutput := func(expectedData []byte, checkFilename func(string)) {
				lo, _, err := o.rationalize(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, lo.args, should.HaveLength(2))
				assert.Loosely(t, lo.args[0], should.Equal(luciexe.OutputCLIArg))
				checkFilename(lo.args[1])

				_, err = luciexe.ReadBuildFile(lo.collectPath)
				assert.Loosely(t, err, should.ErrLike("opening build file"))

				assert.Loosely(t, os.WriteFile(lo.args[1], expectedData, 0666), should.BeNil)

				build, err := luciexe.ReadBuildFile(lo.collectPath)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, build, should.Match(expected))
			}

			t.Run(`collect but no specific file`, func(t *ftt.Test) {
				o.CollectOutput = true
				data, err := proto.Marshal(expected)
				assert.Loosely(t, err, should.BeNil)
				testParseOutput(data, func(filename string) {
					assert.Loosely(t, filename, should.HavePrefix(tdir))
					assert.Loosely(t, filename, should.HaveSuffix(luciexe.BuildFileCodecBinary.FileExtension()))
				})
			})

			t.Run(`collect from a binary file`, func(t *ftt.Test) {
				o.CollectOutput = true
				outPath := filepath.Join(tdir, "output.pb")
				o.CollectOutputPath = outPath

				data, err := proto.Marshal(expected)
				assert.Loosely(t, err, should.BeNil)

				testParseOutput(data, func(filename string) {
					assert.Loosely(t, filename, should.Equal(outPath))
				})
			})

			t.Run(`collect from a json file`, func(t *ftt.Test) {
				o.CollectOutput = true
				outPath := filepath.Join(tdir, "output.json")
				o.CollectOutputPath = outPath

				data, err := (&jsonpb.Marshaler{OrigName: true}).MarshalToString(expected)
				assert.Loosely(t, err, should.BeNil)

				testParseOutput([]byte(data), func(filename string) {
					assert.Loosely(t, filename, should.Equal(outPath))
				})
			})

			t.Run(`collect from a textpb file`, func(t *ftt.Test) {
				o.CollectOutput = true
				outPath := filepath.Join(tdir, "output.textpb")
				o.CollectOutputPath = outPath

				testParseOutput([]byte(expected.String()), func(filename string) {
					assert.Loosely(t, filename, should.Equal(outPath))
				})
			})
		})
	})
}

func TestOptionsEnv(t *testing.T) {
	ftt.Run(`Env`, t, func(t *ftt.Test) {
		ctx, o, _ := commonOptions(t)

		t.Run(`default`, func(t *ftt.Test) {
			lo, _, err := o.rationalize(ctx)
			assert.Loosely(t, err, should.BeNil)

			expected := stringset.NewFromSlice(luciexe.TempDirEnvVars...)
			for key := range o.Env.Map() {
				expected.Add(key)
			}
			expected.Add(lucictx.EnvKey)

			actual := stringset.New(expected.Len())
			for key := range lo.env.Map() {
				actual.Add(key)
			}

			assert.Loosely(t, actual, should.Match(expected))
		})
	})
}

func TestOptionsStdio(t *testing.T) {
	ftt.Run(`stdio`, t, func(t *ftt.Test) {
		ctx, o, _ := commonOptions(t)

		t.Run(`default`, func(t *ftt.Test) {
			lo, _, err := o.rationalize(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, lo.stderr, should.NotBeNil)
			assert.Loosely(t, lo.stdout, should.NotBeNil)
		})

		t.Run(`errors`, func(t *ftt.Test) {
			t.Run(`bad bootstrap (missing)`, func(t *ftt.Test) {
				o.Env.Remove(bootstrap.EnvStreamServerPath)
				_, _, err := o.rationalize(ctx)
				assert.Loosely(t, err, should.ErrLike("Logdog Butler environment required"))
			})

			t.Run(`bad bootstrap (malformed)`, func(t *ftt.Test) {
				o.Env.Set(bootstrap.EnvStreamPrefix, "!!!!")
				_, _, err := o.rationalize(ctx)
				assert.Loosely(t, err, should.ErrLike(`failed to validate prefix "!!!!"`))
			})
		})
	})
}

func TestOptionsExtraDirs(t *testing.T) {
	ftt.Run(`tempDir+workDir`, t, func(t *ftt.Test) {
		ctx, o, tdir := commonOptions(t)

		t.Run(`provided BaseDir`, func(t *ftt.Test) {
			o.BaseDir = filepath.Join(tdir, "base")
			assert.Loosely(t, os.Mkdir(o.BaseDir, 0777), should.BeNil)
			lo, _, err := o.rationalize(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, lo.env.Get("TMP"), should.HavePrefix(o.BaseDir))
			assert.Loosely(t, lo.workDir, should.HavePrefix(o.BaseDir))
		})

		t.Run(`provided BaseDir does not exist`, func(t *ftt.Test) {
			o.BaseDir = filepath.Join(tdir, "base")
			_, _, err := o.rationalize(ctx)
			assert.Loosely(t, err, should.ErrLike("checking BaseDir: dir does not exist"))
		})

		t.Run(`provided BaseDir is not a directory`, func(t *ftt.Test) {
			o.BaseDir = filepath.Join(tdir, "base")
			assert.Loosely(t, os.WriteFile(o.BaseDir, []byte("not a dir"), 0666), should.BeNil)
			_, _, err := o.rationalize(ctx)
			assert.Loosely(t, err, should.ErrLike("checking BaseDir: path is not a directory"))
		})

		t.Run(`fallback to temp`, func(t *ftt.Test) {
			lo, _, err := o.rationalize(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, lo.env.Get("TMP"), should.HavePrefix(tdir))
			assert.Loosely(t, lo.workDir, should.HavePrefix(tdir))
		})
	})
}
