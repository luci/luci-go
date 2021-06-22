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
	"go.chromium.org/luci/cv/internal/common"
)

type pclIndex map[common.CLID]int

func (p pclIndex) addI64(id int64, index int) { p[common.CLID(id)] = index }
func (p pclIndex) hasI64(id int64) bool       { return p.has(common.CLID(id)) }
func (p pclIndex) has(clid common.CLID) bool {
	_, exists := p[clid]
	return exists
}

// ensurePCLIndex constructs and fills pclIndex if necessary.
func (s *State) ensurePCLIndex() {
	if s.pclIndex != nil {
		return
	}
	s.pclIndex = make(pclIndex, len(s.PB.GetPcls()))
	s.forceUpdatePCLIndex()
}

// forceUpdatePCLIndex updates PCLIndex based latest PCLs.
//
// Doesn't delete entries for deleted PCLs, so must be called only after items
// were updated/inserted into PCLs.
func (s *State) forceUpdatePCLIndex() {
	for i, pcl := range s.PB.GetPcls() {
		s.pclIndex.addI64(pcl.GetClid(), i)
	}
}
