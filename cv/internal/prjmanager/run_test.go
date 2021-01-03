// Copyright 2020 The LUCI Authors.
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

package prjmanager

import (
	"encoding/hex"
	"testing"
	"time"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

func TestComputeCLsDigest(t *testing.T) {
	t.Parallel()

	Convey("RunBuilder.computeCLsDigest works", t, func() {
		// This test mirrors the `test_attempt_key_hash` in CQDaemon's
		// pending_manager/test/base_test.py file.
		snapshotOf := func(host string, num int64) *changelist.Snapshot {
			return &changelist.Snapshot{
				Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
					Host: host,
					Info: &gerritpb.ChangeInfo{Number: num},
				}},
			}
		}
		epoch := time.Date(2020, time.December, 31, 0, 0, 0, 0, time.UTC)
		triggerAt := func(delay time.Duration) *run.Trigger {
			return &run.Trigger{Time: timestamppb.New(epoch.Add(delay))}
		}

		rb := RunBuilder{
			InputCLs: []RunBuilderCL{
				{
					Snapshot:    snapshotOf("b.example.com", 1),
					TriggerInfo: triggerAt(49999 * time.Microsecond),
				},
				{
					Snapshot:    snapshotOf("a.example.com", 2),
					TriggerInfo: triggerAt(777777 * time.Microsecond),
				},
			},
		}
		rb.computeCLsDigest()
		So(rb.runIDBuilder.version, ShouldEqual, 1)
		So(hex.EncodeToString(rb.runIDBuilder.digest), ShouldEqual,
			"28cb4b82698febb13483f5b2eb5e3ec19d5d77e9c503d5aefcb2b11a")
	})
}
