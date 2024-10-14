// Copyright 2022 The LUCI Authors.
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

package model

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/deploy/api/modelpb"
)

// service name => version name => traffic portion it gets
type serviceMap map[string]map[string]int32

func TestIntendedMatchesReported(t *testing.T) {
	t.Parallel()

	ftt.Run("GAE", t, func(t *ftt.Test) {
		type testCase struct {
			intended serviceMap
			reported serviceMap
		}
		check := func(tc testCase) bool {
			return appengineIntendedMatchesReported(stateProto(true, tc.intended), stateProto(false, tc.reported))
		}

		assert.Loosely(t, check(testCase{}), should.BeTrue)

		// Ignored extra services and versions.
		assert.Loosely(t, check(testCase{
			intended: serviceMap{
				"svc1": {"ver1": 200, "ver2": 800},
				"svc2": {"ver1": 1000},
			},
			reported: serviceMap{
				"svc1":    {"ver1": 200, "ver2": 800, "ignored": 0},
				"svc2":    {"ver1": 1000, "ignored": 0},
				"ignored": {"ver1": 1000},
			},
		}), should.BeTrue)

		// Moving traffic.
		assert.Loosely(t, check(testCase{
			intended: serviceMap{
				"svc1": {"ver1": 800, "ver2": 200},
				"svc2": {"ver1": 1000},
			},
			reported: serviceMap{
				"svc1": {"ver1": 200, "ver2": 800},
				"svc2": {"ver1": 1000},
			},
		}), should.BeFalse)

		// Deploying new version.
		assert.Loosely(t, check(testCase{
			intended: serviceMap{
				"svc1": {"ver1": 200, "ver2": 800},
				"svc2": {"ver2": 1000},
			},
			reported: serviceMap{
				"svc1": {"ver1": 200, "ver2": 800},
				"svc2": {"ver1": 1000},
			},
		}), should.BeFalse)

		// Deploying new version, but not shifting any traffic to it.
		assert.Loosely(t, check(testCase{
			intended: serviceMap{
				"svc1": {"ver1": 200, "ver2": 800, "ver3": 0},
				"svc2": {"ver1": 1000},
			},
			reported: serviceMap{
				"svc1": {"ver1": 200, "ver2": 800},
				"svc2": {"ver1": 1000},
			},
		}), should.BeFalse)
	})
}

func serviceProto(name string, intended bool, versions map[string]int32) *modelpb.AppengineState_Service {
	var vers []*modelpb.AppengineState_Service_Version
	for ver := range versions {
		verpb := &modelpb.AppengineState_Service_Version{
			Name: ver,
		}
		if intended {
			verpb.IntendedState = &modelpb.AppengineState_Service_Version_IntendedState{}
		} else {
			verpb.CapturedState = &modelpb.AppengineState_Service_Version_CapturedState{}
		}
		vers = append(vers, verpb)
	}
	return &modelpb.AppengineState_Service{
		Name:              name,
		Versions:          vers,
		TrafficSplitting:  modelpb.AppengineState_Service_COOKIE,
		TrafficAllocation: versions,
	}
}

func stateProto(intended bool, services serviceMap) *modelpb.AppengineState {
	var svc []*modelpb.AppengineState_Service
	for name, vers := range services {
		svc = append(svc, serviceProto(name, intended, vers))
	}
	out := &modelpb.AppengineState{Services: svc}
	if intended {
		out.IntendedState = &modelpb.AppengineState_IntendedState{}
	} else {
		out.CapturedState = &modelpb.AppengineState_CapturedState{}
	}
	return out
}
