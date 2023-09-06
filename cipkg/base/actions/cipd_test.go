// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package actions

import (
	"context"
	"testing"

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/internal/testutils"
	"go.chromium.org/luci/common/exec/execmock"

	. "github.com/smartystreets/goconvey/convey"
)

func TestProcessCIPD(t *testing.T) {
	Convey("Test action processor for cipd", t, func() {
		ap := NewActionProcessor()
		pm := testutils.NewMockPackageManage("")

		cipd := &core.ActionCIPDExport{
			EnsureFile: "infra/tools/luci/vpython/linux-amd64 git_revision:98782288dfc349541691a2a5dfc0e44327f22731",
		}

		pkg, err := ap.Process("", pm, &core.Action{
			Name: "url",
			Spec: &core.Action_Cipd{Cipd: cipd},
		})
		So(err, ShouldBeNil)

		checkReexecArg(pkg.Derivation.Args, cipd)
	})
}

func TestExecuteCIPD(t *testing.T) {
	Convey("Test execute action cipd", t, func() {
		ctx := execmock.Init(context.Background())
		uses := execmock.Simple.Mock(ctx, execmock.SimpleInput{})
		out := t.TempDir()

		Convey("Test cipd export", func() {
			a := &core.ActionCIPDExport{
				EnsureFile: "",
			}

			err := ActionCIPDExportExecutor(ctx, a, out)
			So(err, ShouldBeNil)

			{
				usage := uses.Snapshot()
				So(usage, ShouldHaveLength, 1)
				So(usage[0].Args[1:], ShouldEqual, []string{"export", "--root", out, "--ensure-file", "-"})
			}
		})
	})
}

func TestReexecCIPD(t *testing.T) {
	Convey("Test re-execute action processor for cipd", t, func() {
		ctx := execmock.Init(context.Background())
		uses := execmock.Simple.Mock(ctx, execmock.SimpleInput{})
		ap := NewActionProcessor()
		pm := testutils.NewMockPackageManage("")
		out := t.TempDir()

		pkg, err := ap.Process("", pm, &core.Action{
			Name: "url",
			Spec: &core.Action_Cipd{Cipd: &core.ActionCIPDExport{
				EnsureFile: "",
			}},
		})
		So(err, ShouldBeNil)

		runWithDrv(ctx, pkg.Derivation, out)

		{
			usage := uses.Snapshot()
			So(usage, ShouldHaveLength, 1)
			So(usage[0].Args[1:], ShouldEqual, []string{"export", "--root", out, "--ensure-file", "-"})
		}
	})
}
