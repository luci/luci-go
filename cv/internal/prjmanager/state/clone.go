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

package state

import "go.chromium.org/luci/cv/internal/prjmanager/prjpb"

// cloneShallow returns cloned state ready for in-place mutation.
func (s *State) cloneShallow(reasons ...prjpb.LogReason) *State {
	ret := &State{}
	*ret = *s
	if len(reasons) > 0 {
		ret.LogReasons = append(ret.LogReasons, reasons...)
	}

	// Don't use proto.merge to avoid deep copy.
	ret.PB = &prjpb.PState{
		LuciProject:         s.PB.GetLuciProject(),
		Status:              s.PB.GetStatus(),
		ConfigHash:          s.PB.GetConfigHash(),
		ConfigGroupNames:    s.PB.GetConfigGroupNames(),
		Pcls:                s.PB.GetPcls(),
		Components:          s.PB.GetComponents(),
		RepartitionRequired: s.PB.GetRepartitionRequired(),
		CreatedPruns:        s.PB.GetCreatedPruns(),
		NextEvalTime:        s.PB.GetNextEvalTime(),
		PurgingCls:          s.PB.GetPurgingCls(),
	}

	s.alreadyCloned = true
	return ret
}

func (s *State) ensureNotYetCloned() {
	if s.alreadyCloned {
		panic("Incorrect use. This State object has already been cloned. See State doc")
	}
}
