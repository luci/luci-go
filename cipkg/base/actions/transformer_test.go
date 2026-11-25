// Copyright 2026 The LUCI Authors.
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
	"errors"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/types/known/anypb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/testutils"
)

func TestSetTransformer(t *testing.T) {
	ftt.Run("Test set transformer", t, func(t *ftt.Test) {
		ctx := context.Background()
		ap := NewActionProcessor()
		pm := testutils.NewMockPackageManage("")

		t.Run("ok", func(t *ftt.Test) {
			err := SetTransformer(ap, func(ctx context.Context, msg *anypb.Any, deps []Package) (*core.Derivation, error) {
				return &core.Derivation{}, nil
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("duplicated", func(t *ftt.Test) {
			err := SetTransformer(ap, func(ctx context.Context, msg *anypb.Any, deps []Package) (*core.Derivation, error) {
				return &core.Derivation{}, nil
			})
			assert.Loosely(t, err, should.BeNil)
			err = SetTransformer(ap, func(ctx context.Context, msg *anypb.Any, deps []Package) (*core.Derivation, error) {
				return &core.Derivation{}, nil
			})
			assert.Loosely(t, errors.Is(err, ErrTransformerExisted), should.BeTrue)
		})

		t.Run("sealed", func(t *ftt.Test) {
			_, err := ap.Process(ctx, "", pm, &core.Action{
				Name: "cmd",
				Spec: &core.Action_Command{Command: &core.ActionCommand{}},
			})
			assert.Loosely(t, err, should.BeNil)
			err = SetTransformer(ap, func(ctx context.Context, msg *anypb.Any, deps []Package) (*core.Derivation, error) {
				return &core.Derivation{}, nil
			})
			assert.Loosely(t, errors.Is(err, ErrActionProcessorSealed), should.BeTrue)
		})

		t.Run("sealed by transform", func(t *ftt.Test) {
			_, err := ap.Transform(ctx, "", &core.Action{
				Name: "cmd",
				Spec: &core.Action_Command{Command: &core.ActionCommand{}},
			}, nil)
			assert.Loosely(t, err, should.BeNil)
			err = SetTransformer(ap, func(ctx context.Context, msg *anypb.Any, deps []Package) (*core.Derivation, error) {
				return &core.Derivation{}, nil
			})
			assert.Loosely(t, errors.Is(err, ErrActionProcessorSealed), should.BeTrue)
		})
	})
}

func TestTransform(t *testing.T) {
	ftt.Run("Test transform", t, func(t *ftt.Test) {
		ctx := context.Background()
		ap := NewActionProcessor()
		pm := testutils.NewMockPackageManage("/storage")

		dep := Package{
			Action:       &core.Action{Name: "dep"},
			DerivationID: "dep-customid",
			Handler:      pm.Get("dep-customid"),
		}
		drv, err := ap.Transform(ctx, "linux-amd64", &core.Action{
			Name: "pkg",
			Spec: &core.Action_Command{
				Command: &core.ActionCommand{
					Args: []string{"bin", DepRef("dep")},
				},
			},
		}, []Package{dep})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, drv.Name, should.Equal("pkg"))
		assert.Loosely(t, drv.Platform, should.Equal("linux-amd64"))
		assert.Loosely(t, drv.Inputs, should.Match([]string{"dep-customid"}))
		assert.Loosely(t, drv.Args, should.Match([]string{"bin", dep.Handler.OutputDirectory()}))
	})

	ftt.Run("Test ActionRef", t, func(t *ftt.Test) {
		ctx := context.Background()
		ap := NewActionProcessor()
		pm := testutils.NewMockPackageManage("/storage")

		target := &core.Action{
			Name: "runc_url_deadbeef",
			Metadata: &core.Action_Metadata{
				Cipd: &core.Action_Metadata_CIPD{Name: "pkg/fetch"},
			},
			Spec: &core.Action_Command{
				Command: &core.ActionCommand{Args: []string{"fetch"}},
			},
		}
		ref := &core.Action{
			Name: "runc",
			Metadata: &core.Action_Metadata{
				Cipd: &core.Action_Metadata_CIPD{Name: "pkg/linux-amd64"},
			},
			Deps: []*core.Action{target},
			Spec: &core.Action_Ref{Ref: &core.ActionRef{}},
		}

		t.Run("preserves target derivation and derivation ID", func(t *ftt.Test) {
			targetPkg, err := ap.Process(ctx, "linux-amd64", pm, target)
			assert.NoErr(t, err)
			refPkg, err := ap.Process(ctx, "linux-amd64", pm, ref)
			assert.NoErr(t, err)

			assert.Loosely(t, refPkg.Derivation, should.Match(targetPkg.Derivation))
			assert.Loosely(t, refPkg.DerivationID, should.Equal(targetPkg.DerivationID))
			assert.Loosely(t, refPkg.ActionID, should.NotEqual(targetPkg.ActionID))
			assert.Loosely(t, refPkg.Handler.OutputDirectory(), should.Equal(targetPkg.Handler.OutputDirectory()))
		})

		t.Run("dependents reference ref by its action name", func(t *ftt.Test) {
			consumer := &core.Action{
				Name: "consumer",
				Deps: []*core.Action{ref},
				Spec: &core.Action_Command{
					Command: &core.ActionCommand{
						Args: []string{DepRef("runc", "bin", "runc")},
					},
				},
			}
			pkg, err := ap.Process(ctx, "linux-amd64", pm, consumer)
			assert.NoErr(t, err)
			refPkg := pkg.BuildDependencies[0]
			assert.Loosely(t, pkg.Derivation.Args, should.Match([]string{
				filepath.Join(refPkg.Handler.OutputDirectory(), "bin", "runc"),
			}))
		})

		t.Run("requires exactly one dependency", func(t *ftt.Test) {
			_, err := ap.Process(ctx, "linux-amd64", pm, &core.Action{
				Name: "bad_zero",
				Spec: &core.Action_Ref{Ref: &core.ActionRef{}},
			})
			assert.Loosely(t, err, should.ErrLike("must have exactly 1 dependency"))

			_, err = ap.Process(ctx, "linux-amd64", pm, &core.Action{
				Name: "bad_two",
				Deps: []*core.Action{target, target},
				Spec: &core.Action_Ref{Ref: &core.ActionRef{}},
			})
			assert.Loosely(t, err, should.ErrLike("must have exactly 1 dependency"))
		})
	})
}
