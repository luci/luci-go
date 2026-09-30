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
	"errors"
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
		ap := NewActionProcessor()
		pm := testutils.NewMockPackageManage("")

		t.Run("ok", func(t *ftt.Test) {
			err := SetTransformer(ap, func(msg *anypb.Any, deps []Package) (*core.Derivation, error) {
				return &core.Derivation{}, nil
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("duplicated", func(t *ftt.Test) {
			err := SetTransformer(ap, func(msg *anypb.Any, deps []Package) (*core.Derivation, error) {
				return &core.Derivation{}, nil
			})
			assert.Loosely(t, err, should.BeNil)
			err = SetTransformer(ap, func(msg *anypb.Any, deps []Package) (*core.Derivation, error) {
				return &core.Derivation{}, nil
			})
			assert.Loosely(t, errors.Is(err, ErrTransformerExisted), should.BeTrue)
		})

		t.Run("sealed", func(t *ftt.Test) {
			_, err := ap.Process("", pm, &core.Action{
				Name: "cmd",
				Spec: &core.Action_Command{Command: &core.ActionCommand{}},
			})
			assert.Loosely(t, err, should.BeNil)
			err = SetTransformer(ap, func(msg *anypb.Any, deps []Package) (*core.Derivation, error) {
				return &core.Derivation{}, nil
			})
			assert.Loosely(t, errors.Is(err, ErrActionProcessorSealed), should.BeTrue)
		})
	})
}

func TestSetPostProcessor(t *testing.T) {
	ftt.Run("Test set post processor", t, func(t *ftt.Test) {
		ap := NewActionProcessor()
		pm := testutils.NewMockPackageManage("")

		t.Run("ok", func(t *ftt.Test) {
			var processed []string
			err := ap.SetPostProcessor(func(a *core.Action) error {
				processed = append(processed, a.Name)
				a.Metadata = &core.Action_Metadata{
					CacheId: "cache-" + a.Name,
				}
				return nil
			})
			assert.Loosely(t, err, should.BeNil)

			pkg, err := ap.Process("", pm, &core.Action{
				Name: "root",
				Deps: []*core.Action{
					{Name: "dep", Spec: &core.Action_Command{Command: &core.ActionCommand{}}},
				},
				Spec: &core.Action_Command{Command: &core.ActionCommand{}},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, processed, should.Match([]string{"dep", "root"}))
			assert.Loosely(t, pkg.Action.GetMetadata().GetCacheId(), should.Equal("cache-root"))
			assert.Loosely(t, pkg.BuildDependencies[0].Action.GetMetadata().GetCacheId(), should.Equal("cache-dep"))
		})

		t.Run("sealed", func(t *ftt.Test) {
			_, err := ap.Process("", pm, &core.Action{
				Name: "cmd",
				Spec: &core.Action_Command{Command: &core.ActionCommand{}},
			})
			assert.Loosely(t, err, should.BeNil)
			err = ap.SetPostProcessor(func(a *core.Action) error { return nil })
			assert.Loosely(t, errors.Is(err, ErrActionProcessorSealed), should.BeTrue)
		})

		t.Run("clone", func(t *ftt.Test) {
			var called int
			ap.MustSetPostProcessor(func(a *core.Action) error {
				called++
				return nil
			})
			_, err := ap.Process("", pm, &core.Action{
				Name: "cmd",
				Spec: &core.Action_Command{Command: &core.ActionCommand{}},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, called, should.Equal(1))

			cloned := ap.Clone()
			cloned.MustSetPostProcessor(nil)
			_, err = cloned.Process("", pm, &core.Action{
				Name: "cmd",
				Spec: &core.Action_Command{Command: &core.ActionCommand{}},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, called, should.Equal(1))
		})

		t.Run("error", func(t *ftt.Test) {
			expectedErr := errors.New("post process failed")
			ap.MustSetPostProcessor(func(a *core.Action) error {
				return expectedErr
			})
			_, err := ap.Process("", pm, &core.Action{
				Name: "cmd",
				Spec: &core.Action_Command{Command: &core.ActionCommand{}},
			})
			assert.Loosely(t, errors.Is(err, expectedErr), should.BeTrue)
		})
	})
}
