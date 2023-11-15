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

import (
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit/cfgmatcher"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

// State is a state of Project Manager.
//
// The state object must not be re-used except for serializing public state
// after its public methods returned a modified State or an error.
// This allows for efficient evolution of cached helper datastructures which
// would otherwise have to be copied, too.
//
// To illustrate correct and incorrect usages:
//
//	s0 := &State{...}
//	s1, _, err := s0.Mut1()
//	if err != nil {
//	  // ... := s0.Mut2()             // NOT OK, 2nd call on s0
//	  return proto.Marshal(s0.PB)     // OK
//	}
//	//  ... := s0.Mut2()              // NOT OK, 2nd call on s0
//	s2, _, err := s1.Mut2()           // OK, s1 may be s0 if Mut1() was noop
//	if err != nil {
//	  // return proto.Marshal(s0.PB)  // OK
//	  return proto.Marshal(s0.PB)     // OK
//	}
type State struct {
	// PB is the serializable part of State mutated using copy-on-write approach
	// https://en.wikipedia.org/wiki/Copy-on-write
	PB *prjpb.PState

	// LogReasons is append-only accumulation of reasons to record this state for
	// posterity.
	LogReasons []prjpb.LogReason

	// Helper private fields used during mutations.

	// alreadyCloned is set to true after state is cloned to prevent incorrect
	// usage.
	alreadyCloned bool
	// configGroups are cacehd config groups.
	configGroups []*prjcfg.ConfigGroup
	// cfgMatcher is lazily created, cached, and passed on to State clones.
	cfgMatcher *cfgmatcher.Matcher
	// pclIndex provides O(1) check if PCL exists for a CL.
	//
	// lazily created, see ensurePCLIndex().
	pclIndex pclIndex // CLID => index in PB.Pcls slice.
}

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
		TriggeringCls:       s.PB.GetTriggeringCls(),
		TriggeringClDeps:    s.PB.GetTriggeringClDeps(),
	}

	s.alreadyCloned = true
	return ret
}

func (s *State) ensureNotYetCloned() {
	if s.alreadyCloned {
		panic("Incorrect use. This State object has already been cloned. See State doc")
	}
}

// UpgradeIfNecessary upgrades old state to new format if necessary.
//
// Returns the new state, or this state if nothing was changed.
func (s *State) UpgradeIfNecessary() *State {
	if !s.needUpgrade() {
		return s
	}
	s = s.cloneShallow()
	s.upgrade()
	return s
}
