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
