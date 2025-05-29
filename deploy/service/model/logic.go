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
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/deploy/api/modelpb"
)

// ValidateIntendedState checks AssetState proto matches the asset kind.
//
// Checks all `intended_state` fields are populated. Purely erronous states
// (with no state and non-zero status code) are considered valid.
func ValidateIntendedState(assetID string, state *modelpb.AssetState) error {
	return validateState(assetID, state, func(s any) error {
		switch state := s.(type) {
		case *modelpb.AppengineState:
			return validateAppengineIntendedState(state)
		default:
			panic("impossible")
		}
	})
}

// ValidateReportedState checks AssetState proto matches the asset kind.
//
// Checks all `captured_state` fields are populated. Purely erronous states
// (with no state and non-zero status code) are considered valid.
func ValidateReportedState(assetID string, state *modelpb.AssetState) error {
	return validateState(assetID, state, func(s any) error {
		switch state := s.(type) {
		case *modelpb.AppengineState:
			return validateAppengineReportedState(state)
		default:
			panic("impossible")
		}
	})
}

// validateState verifies assetID kind matches the populated oneof field in
// `state` and calls the callback, passing this oneof's payload to it.
func validateState(assetID string, state *modelpb.AssetState, cb func(state any) error) error {
	switch {
	case state == nil:
		// AssetState itself should be populated.
		return errors.New("no state populated")
	case state.Status.GetCode() != int32(codes.OK):
		if state.State != nil {
			return errors.New("if `status` is not OK, `state` should be absent")
		}
		return nil
	case isAppengineAssetID(assetID):
		if s := state.GetAppengine(); s != nil {
			return cb(s)
		}
		return errors.New("not an Appengine state")
	default:
		return errors.New("unrecognized asset ID format")
	}
}

// IsActuationEnabed checks if the actuation for an asset is enabled.
func IsActuationEnabed(cfg *modelpb.AssetConfig, dep *modelpb.DeploymentConfig) bool {
	return cfg.GetEnableAutomation()
}

// IntendedMatchesReported is true if the intended state matches the reported
// state.
//
// States must be non-erroneous and be valid per ValidateIntendedState and
// ValidateReportedState.
func IntendedMatchesReported(intended, reported *modelpb.AssetState) bool {
	if intended := intended.GetAppengine(); intended != nil {
		if reported := reported.GetAppengine(); reported != nil {
			return appengineIntendedMatchesReported(intended, reported)
		}
		return false
	}
	return false
}

// IsSameState compares `state` portion of AssetState.
//
// Ignores all other fields. If any of the states is erroneous (with no `state`
// field), returns false.
func IsSameState(a, b *modelpb.AssetState) bool {
	// Note that `state` is a oneof field and there's no way to compare such
	// fields without examining "arms" first.
	concrete := func(s *modelpb.AssetState) proto.Message {
		switch v := s.GetState().(type) {
		case *modelpb.AssetState_Appengine:
			return v.Appengine
		default:
			return nil
		}
	}
	if a, b := concrete(a), concrete(b); a != nil && b != nil {
		return proto.Equal(a, b)
	}
	return false
}

// IsUpToDate returns true if an asset is up-to-date and should not be actuated.
//
// An asset is considered up-to-date if its reported state matches the intended
// state, and the intended state hasn't changed since the last successful
// actuation.
func IsUpToDate(inteded, reported, applied *modelpb.AssetState) bool {
	return applied != nil &&
		IntendedMatchesReported(inteded, reported) &&
		IsSameState(inteded, applied)
}

////////////////////////////////////////////////////////////////////////////////
// Appengine logic.

func isAppengineAssetID(assetID string) bool {
	return strings.HasPrefix(assetID, "apps/")
}

func validateAppengineIntendedState(state *modelpb.AppengineState) error {
	if state.IntendedState == nil {
		return errors.New("no intended_state field")
	}

	err := visitServices(state, true, func(svc *modelpb.AppengineState_Service) error {
		if err := validateTrafficAllocation(svc.TrafficAllocation); err != nil {
			return err
		}
		if svc.TrafficSplitting == 0 {
			return errors.New("no traffic_splitting field")
		}
		return nil
	})
	if err != nil {
		return err
	}

	return visitVersions(state, true, func(ver *modelpb.AppengineState_Service_Version) error {
		if ver.IntendedState == nil {
			return errors.New("no intended_state field")
		}
		return nil
	})
}

func validateAppengineReportedState(state *modelpb.AppengineState) error {
	if state.CapturedState == nil {
		return errors.New("no captured_state field")
	}

	// Note: the list of reported services may be empty for a completely new GAE
	// app.
	err := visitServices(state, true, func(svc *modelpb.AppengineState_Service) error {
		if err := validateTrafficAllocation(svc.TrafficAllocation); err != nil {
			return err
		}
		// Sadly, GAE Admin API doesn't report traffic_splitting method, so we skip
		// validating it.
		return nil
	})
	if err != nil {
		return err
	}

	// Note: it is not possible to have a GAE service running without any
	// versions.
	return visitVersions(state, false, func(ver *modelpb.AppengineState_Service_Version) error {
		if ver.CapturedState == nil {
			return errors.New("no captured_state field")
		}
		return nil
	})
}

func validateTrafficAllocation(t map[string]int32) error {
	if len(t) == 0 {
		return errors.New("no traffic_allocation field")
	}
	total := 0
	for _, p := range t {
		total += int(p)
	}
	if total != 1000 {
		return errors.Fmt("traffic_allocation: total traffic %d != 1000", total)
	}
	return nil
}

// visitServices calls the callback for each Service proto.
func visitServices(state *modelpb.AppengineState, allowEmpty bool, cb func(*modelpb.AppengineState_Service) error) error {
	if len(state.Services) == 0 && !allowEmpty {
		return errors.New("services list is empty")
	}
	for _, svc := range state.Services {
		if svc.Name == "" {
			return errors.New("unnamed service")
		}
		if err := cb(svc); err != nil {
			return errors.Fmt("in service %q: %w", svc.Name, err)
		}
	}
	return nil
}

// visitVersions calls the callback for each Version proto across all Services.
func visitVersions(state *modelpb.AppengineState, allowEmpty bool, cb func(*modelpb.AppengineState_Service_Version) error) error {
	for _, svc := range state.Services {
		if len(svc.Versions) == 0 && !allowEmpty {
			return errors.Fmt("in service %q: no versions", svc.Name)
		}
		for _, ver := range svc.Versions {
			if ver.Name == "" {
				return errors.Fmt("in service %q: unnamed version", svc.Name)
			}
			if err := cb(ver); err != nil {
				return errors.Fmt("in service %q: in version %q: %w", svc.Name, ver.Name, err)
			}
		}
	}
	return nil
}

// appengineIntendedMatchesReported returns true if all intended versions are
// deployed and receive the intended percent of traffic.
//
// It is OK if more versions or services are deployed as long as they don't get
// any traffic.
//
// Note that Appengine Admin API doesn't provide visibility into what versions
// of "special" YAMLs (like queue.yaml and index.yaml) are deployed. They are
// ignored by this function. Same applies to the traffic splitting method.
func appengineIntendedMatchesReported(intended, reported *modelpb.AppengineState) bool {
	if err := validateAppengineIntendedState(intended); err != nil {
		panic(fmt.Sprintf("got invalid intended state (%s): %q", err, intended))
	}
	if err := validateAppengineReportedState(reported); err != nil {
		panic(fmt.Sprintf("got invalid reported state (%s): %s", err, reported))
	}

	want := trafficMap(intended)
	have := trafficMap(reported)

	for svc, wantTraffic := range want {
		// Note: `wantTraffic` may be 0 if we want to deploy a version, but don't
		// route any traffic to it yet.
		switch currentTraffic, ok := have[svc]; {
		case !ok:
			// No such version deployed at all.
			return false
		case currentTraffic != wantTraffic:
			// Deployed, but receives wrong portion of traffic.
			return false
		}
	}

	return true
}

type serviceVersion struct {
	service string // e.g. "default"
	version string // e.g. "24344-abcedfa"
}

// trafficMap returns a mapping from a concrete version to the portion of
// traffic assigned to it within its service.
func trafficMap(s *modelpb.AppengineState) map[serviceVersion]int {
	out := make(map[serviceVersion]int)
	for _, svc := range s.Services {
		for _, ver := range svc.Versions {
			out[serviceVersion{svc.Name, ver.Name}] = int(svc.TrafficAllocation[ver.Name])
		}
	}
	return out
}
