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

	"go.chromium.org/luci/common/exec/execmock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/internal/testutils"
)

func TestProcessCIPD(t *testing.T) {
	ftt.Run("Test action processor for cipd", t, func(t *ftt.Test) {
		ap := NewActionProcessor()
		pm := testutils.NewMockPackageManage("")

		cipd := &core.ActionCIPDExport{
			EnsureFile: "infra/tools/luci/vpython/linux-amd64 git_revision:98782288dfc349541691a2a5dfc0e44327f22731",
		}

		pkg, err := ap.Process("", pm, &core.Action{
			Name: "url",
			Spec: &core.Action_Cipd{Cipd: cipd},
		})
		assert.Loosely(t, err, should.BeNil)

		checkReexecArg(t, pkg.Derivation.Args, cipd)
	})
}

func TestExecuteCIPD(t *testing.T) {
	ftt.Run("Test execute action cipd", t, func(t *ftt.Test) {
		ctx := execmock.Init(context.Background())
		uses := execmock.Simple.Mock(ctx, execmock.SimpleInput{})
		out := t.TempDir()

		t.Run("Test cipd export", func(t *ftt.Test) {
			a := &core.ActionCIPDExport{
				EnsureFile: "",
			}

			err := ActionCIPDExportExecutor(ctx, a, out)
			assert.Loosely(t, err, should.BeNil)

			{
				usage := uses.Snapshot()
				assert.Loosely(t, usage, should.HaveLength(1))
				assert.Loosely(t, usage[0].Args[1:], should.Match([]string{"export", "--root", out, "--ensure-file", "-"}))
			}
		})
	})
}

func TestReexecCIPD(t *testing.T) {
	ftt.Run("Test re-execute action processor for cipd", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)

		runWithDrv(t, ctx, pkg.Derivation, out)

		{
			usage := uses.Snapshot()
			assert.Loosely(t, usage, should.HaveLength(1))
			assert.Loosely(t, usage[0].Args[1:], should.Match([]string{"export", "--root", out, "--ensure-file", "-"}))
		}
	})
}
