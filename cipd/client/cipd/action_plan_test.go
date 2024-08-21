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

package cipd

import (
	"testing"

	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPerPinActions(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		p1 := common.Pin{PackageName: "p", InstanceID: "1"}
		p2 := common.Pin{PackageName: "p", InstanceID: "2"}
		p3 := common.Pin{PackageName: "p", InstanceID: "3"}
		p4 := common.Pin{PackageName: "p", InstanceID: "4"}
		p5 := common.Pin{PackageName: "p", InstanceID: "5"}
		p6 := common.Pin{PackageName: "p", InstanceID: "6"}
		p7 := common.Pin{PackageName: "p", InstanceID: "7"}

		reinstallPlan := RepairPlan{NeedsReinstall: true}
		relinkPlan := RepairPlan{ToRelink: []string{"a"}}
		repairPlan := RepairPlan{ToRedeploy: []string{"b"}}

		am := ActionMap{
			"": {
				ToInstall: common.PinSlice{p1},
				ToUpdate: []UpdatedPin{
					{From: p2, To: p3},
				},
				ToRemove: common.PinSlice{p4},
				ToRepair: []BrokenPin{
					{Pin: p5, RepairPlan: reinstallPlan},
					{Pin: p6, RepairPlan: relinkPlan},
					{Pin: p7, RepairPlan: repairPlan},
				},
			},
			"a": {
				ToInstall: common.PinSlice{p2},
				ToUpdate: []UpdatedPin{
					{From: p3, To: p4},
				},
				ToRemove: common.PinSlice{p5},
				ToRepair: []BrokenPin{
					{Pin: p6, RepairPlan: reinstallPlan},
					{Pin: p7, RepairPlan: relinkPlan},
					{Pin: p1, RepairPlan: repairPlan},
				},
			},
			"b": {
				ToInstall: common.PinSlice{p3},
				ToUpdate: []UpdatedPin{
					{From: p4, To: p5},
				},
				ToRemove: common.PinSlice{p6},
				ToRepair: []BrokenPin{
					{Pin: p7, RepairPlan: reinstallPlan},
					{Pin: p1, RepairPlan: relinkPlan},
					{Pin: p2, RepairPlan: repairPlan},
				},
			},
		}

		assert.Loosely(t, am.perPinActions(), should.Resemble(perPinActions{
			maintenance: []pinAction{
				{action: ActionRemove, pin: p4},
				{action: ActionRelink, pin: p6, repairPlan: relinkPlan},
				{action: ActionRemove, pin: p5, subdir: "a"},
				{action: ActionRelink, pin: p7, repairPlan: relinkPlan, subdir: "a"},
				{action: ActionRemove, pin: p6, subdir: "b"},
				{action: ActionRelink, pin: p1, repairPlan: relinkPlan, subdir: "b"},
			},
			updates: []updateActions{
				{
					pin: p1,
					updates: []pinAction{
						{action: ActionInstall, pin: p1},
						{action: ActionRepair, pin: p1, repairPlan: repairPlan, subdir: "a"},
					},
				},
				{
					pin: p3,
					updates: []pinAction{
						{action: ActionInstall, pin: p3},
						{action: ActionInstall, pin: p3, subdir: "b"},
					},
				},
				{
					pin: p5,
					updates: []pinAction{
						{action: ActionInstall, pin: p5, repairPlan: reinstallPlan},
						{action: ActionInstall, pin: p5, subdir: "b"},
					},
				},
				{
					pin: p7,
					updates: []pinAction{
						{action: ActionRepair, pin: p7, repairPlan: repairPlan},
						{action: ActionInstall, pin: p7, repairPlan: reinstallPlan, subdir: "b"},
					},
				},
				{
					pin: p2,
					updates: []pinAction{
						{action: ActionInstall, pin: p2, subdir: "a"},
						{action: ActionRepair, pin: p2, repairPlan: repairPlan, subdir: "b"},
					},
				},
				{
					pin: p4,
					updates: []pinAction{
						{action: ActionInstall, pin: p4, subdir: "a"},
					},
				},
				{
					pin: p6,
					updates: []pinAction{
						{action: ActionInstall, pin: p6, repairPlan: reinstallPlan, subdir: "a"},
					},
				},
			},
		}))
	})
}
