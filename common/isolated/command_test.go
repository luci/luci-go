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

package isolated

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.chromium.org/luci/common/system/environ"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReplaceParameters(t *testing.T) {
	t.Parallel()

	Convey("replaceParameters", t, func() {
		ctx := context.Background()

		Convey("test EXECUTABLE_SUFFIX", func() {
			arg, err := replaceParameters(ctx, "program${EXECUTABLE_SUFFIX}", "", "")
			So(err, ShouldBeNil)
			if runtime.GOOS == "windows" {
				So(arg, ShouldEqual, "program.exe")
			} else {
				So(arg, ShouldEqual, "program")
			}
		})

		Convey("test ISOLATED_OUTDIR", func() {
			arg, err := replaceParameters(ctx, "${ISOLATED_OUTDIR}/result.txt", "out", "")
			So(err, ShouldBeNil)
			So(arg, ShouldEqual, filepath.Join("out", "result.txt"))
		})

		Convey("test SWARMING_BOT_FILE", func() {
			arg, err := replaceParameters(ctx, "${SWARMING_BOT_FILE}/config", "", "cfgdir")
			So(err, ShouldBeNil)
			So(arg, ShouldEqual, filepath.Join("cfgdir", "config"))
		})
	})
}

func TestProcessCommand(t *testing.T) {
	t.Parallel()

	Convey("processCommand", t, func() {
		ctx := context.Background()
		args, err := processCommand(ctx, []string{
			"program${EXECUTABLE_SUFFIX}",
			"${ISOLATED_OUTDIR}/result.txt",
			"${SWARMING_BOT_FILE}/config",
		}, "out", "cfgdir")

		So(err, ShouldBeNil)

		executableSuffix := ""
		if runtime.GOOS == "windows" {
			executableSuffix = ".exe"
		}

		So(args, ShouldResemble, []string{
			"program" + executableSuffix,
			filepath.Join("out", "result.txt"),
			filepath.Join("cfgdir", "config"),
		})
	})
}

func TestGetCommandEnv(t *testing.T) {
	t.Parallel()
	originalEnvironSystem := environSystem
	defer func() {
		environSystem = originalEnvironSystem
	}()

	Convey("GetCommandEnv", t, func() {
		environSystem = func() environ.Env {
			return environ.New([]string{
				"C=foo",
				"D=bar",
				"E=baz",
				"PATH=/bin",
			})
		}

		Convey("simple case", func() {
			env, err := getCommandEnv(context.Background(), "/a", nil, "/b", environ.New([]string{
				"A=a",
				"B=",
				"C=",
				"E=${ISOLATED_OUTDIR}/eggs",
			}), map[string][]string{"D": {"foo"}}, "/spam", "")

			So(err, ShouldBeNil)

			_, ok := env.Get("B")
			So(ok, ShouldBeFalse)

			_, ok = env.Get("C")
			So(ok, ShouldBeFalse)

			if runtime.GOOS == "windows" {
				So(env.GetEmpty("D"), ShouldEqual, `\b\foo;bar`)
			} else {
				So(env.GetEmpty("D"), ShouldEqual, "/b/foo:bar")
			}

			So(env.GetEmpty("E"), ShouldEqual, string(filepath.Separator)+filepath.Join("spam", "eggs"))
		})

		Convey("cipdInfo", func() {
			env, err := getCommandEnv(context.Background(), "tmp", &cipdInfo{
				binaryPath: "cipddir/cipd",
				cacheDir:   ".cipd/cache",
			}, "", environ.Env{}, nil, "", "")
			So(err, ShouldBeNil)

			expected := map[string]string{
				"C":              "foo",
				"CIPD_CACHE_DIR": ".cipd/cache",
				"D":              "bar",
				"E":              "baz",
				"PATH": strings.Join([]string{"cipddir", "/bin"},
					string(filepath.ListSeparator)),
				"TMPDIR": "tmp",
			}

			if runtime.GOOS == "windows" {
				expected["TMP"] = "tmp"
				expected["TEMP"] = "tmp"
			} else if runtime.GOOS == "darwin" {
				expected["MAC_CHROMIUM_TMPDIR"] = "tmp"
			}

			So(env.Map(), ShouldResemble, expected)
		})
	})
}
