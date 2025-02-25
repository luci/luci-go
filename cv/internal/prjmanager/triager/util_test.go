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

package triager

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

// simplePMState implements itriager.PMState and is used in tests of this
// package.
type simplePMState struct {
	pb  *prjpb.PState
	cgs []*prjcfg.ConfigGroup
}

func (s *simplePMState) PCL(clid int64) *prjpb.PCL {
	for _, pcl := range s.pb.GetPcls() {
		if pcl.GetClid() == clid {
			return pcl
		}
	}
	return nil
}

func (s *simplePMState) PurgingCL(clid int64) *prjpb.PurgingCL {
	for _, p := range s.pb.GetPurgingCls() {
		if p.GetClid() == clid {
			return p
		}
	}
	return nil
}

func (s *simplePMState) TriggeringCLDeps(clid int64) *prjpb.TriggeringCLDeps {
	for _, p := range s.pb.GetTriggeringClDeps() {
		if p.GetOriginClid() == clid {
			return p
		}
	}
	return nil
}

func (s *simplePMState) ConfigGroup(index int32) *prjcfg.ConfigGroup {
	return s.cgs[index]
}

func TestEarliest(t *testing.T) {
	t.Parallel()

	ftt.Run("earliest of two works", t, func(t *ftt.Test) {
		zero := time.Time{}
		epoch := time.Date(2021, time.February, 6, 15, 0, 0, 0, time.UTC)
		after := epoch.Add(time.Hour)
		before := epoch.Add(-time.Hour)

		assert.Loosely(t, earliest(), should.Match(zero))
		assert.Loosely(t, earliest(zero), should.Match(zero))
		assert.Loosely(t, earliest(epoch), should.Match(epoch))

		assert.Loosely(t, earliest(zero, epoch), should.Match(epoch))
		assert.Loosely(t, earliest(epoch, zero), should.Match(epoch))

		assert.Loosely(t, earliest(after, zero, epoch), should.Match(epoch))
		assert.Loosely(t, earliest(epoch, before, zero, after), should.Match(before))
	})
}
