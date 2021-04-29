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

package state

import (
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

func (s *State) needUpgrade() bool {
	for _, pcl := range s.PB.GetPcls() {
		if pcl.GetOwnerLacksEmail() {
			return true
		}
	}
	return false
}

// upgrade upgrades in place via copy-on-write.
func (s *State) upgrade() {
	s.PB.Pcls, _ = s.PB.COWPCLs(func(pcl *prjpb.PCL) *prjpb.PCL {
		if pcl.GetOwnerLacksEmail() {
			pcl = proto.Clone(pcl).(*prjpb.PCL)
			pcl.OwnerLacksEmail = false
			pcl.Errors = append(pcl.Errors, &prjpb.CLError{
				Kind: &prjpb.CLError_OwnerLacksEmail{OwnerLacksEmail: true},
			})
		}
		return pcl
	}, nil)
}
