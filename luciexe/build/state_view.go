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

package build

import (
	bbpb "go.chromium.org/luci/buildbucket/proto"
)

// View is a window into the build State.
//
// You can obtain/manipulate this with the State.Modify method.
type View struct{}

// Modify allows you to atomically manipulate the View on the build State.
//
// Blocking in Modify will block other callers of Modify and Set*, as well as
// the ability for the build State to be sent (with the function set by
// OptSend).
//
// The Set* methods should be preferred unless you need to read/modify/write
// View items.
func (*State) Modify(func(*View)) {
	panic("implement")
}

// SetSummaryMarkdown atomically sets the build's SummaryMarkdown field.
func (*State) SetSummaryMarkdown(string) {
	panic("implement")
}

// SetCritical atomically sets the build's Critical field.
func (*State) SetCritical(bbpb.Trinary) {
	panic("implement")
}

// SetGitilesCommit atomically sets the GitilesCommit.
//
// This will make a copy of the GitilesCommit object to store in the build
// State.
func (*State) SetGitilesCommit(*bbpb.GitilesCommit) {
	panic("implement")
}
