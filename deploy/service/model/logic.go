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
	"strings"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/deploy/api/modelpb"
)

// ValidateIntendedState checks AssetState proto matches the asset kind.
//
// Checks all `intended_state` fields are populated. Purely erronous states
// (with no state and non-zero status code) are considered valid.
func ValidateIntendedState(assetID string, state *modelpb.AssetState) error {
	return validateState(assetID, state, func(s interface{}) error {
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
	return validateState(assetID, state, func(s interface{}) error {
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
func validateState(assetID string, state *modelpb.AssetState, cb func(state interface{}) error) error {
	switch {
	case state == nil:
		// AssetState itself should be populated.
		return errors.Reason("no state populated").Err()
	case state.Status.GetCode() != int32(codes.OK) && state.State == nil:
		// A state with just an error code is valid regardless of asset ID.
		return nil
	case isAppengineAssetID(assetID):
		if s := state.GetAppengine(); s != nil {
			return cb(s)
		}
		return errors.Reason("not an Appengine state").Err()
	default:
		return errors.Reason("unrecognized asset ID format").Err()
	}
}

// IsActuationEnabed checks if the actuation for an asset is enabled.
func IsActuationEnabed(cfg *modelpb.AssetConfig, dep *modelpb.DeploymentConfig) bool {
	return cfg.GetEnableAutomation()
}

// IsUpToDate is true if the intended state matches the reported state.
func IsUpToDate(intended *modelpb.AssetState, reported *modelpb.AssetState) bool {
	if intended := intended.GetAppengine(); intended != nil {
		if reported := reported.GetAppengine(); reported != nil {
			return isAppengineUpToDate(intended, reported)
		}
		return false
	}
	return false
}

////////////////////////////////////////////////////////////////////////////////
// Appengine logic.

func isAppengineAssetID(assetID string) bool {
	return strings.HasPrefix(assetID, "apps/")
}

func validateAppengineIntendedState(state *modelpb.AppengineState) error {
	if state.IntendedState == nil {
		return errors.Reason("no intended_state field").Err()
	}

	err := visitServices(state, func(svc *modelpb.AppengineState_Service) error {
		if len(svc.TrafficAllocation) == 0 {
			return errors.Reason("no traffic_allocation field").Err()
		}
		if svc.TrafficSplitting == 0 {
			return errors.Reason("no traffic_splitting field").Err()
		}
		return nil
	})
	if err != nil {
		return err
	}

	return visitVersions(state, func(ver *modelpb.AppengineState_Service_Version) error {
		if ver.IntendedState == nil {
			return errors.Reason("no intended_state field").Err()
		}
		return nil
	})
}

func validateAppengineReportedState(state *modelpb.AppengineState) error {
	if state.CapturedState == nil {
		return errors.Reason("no captured_state field").Err()
	}

	err := visitServices(state, func(svc *modelpb.AppengineState_Service) error {
		if len(svc.TrafficAllocation) == 0 {
			return errors.Reason("no traffic_allocation field").Err()
		}
		// Sadly, GAE Admin API doesn't report traffic_splitting method, so we skip
		// validating it.
		return nil
	})
	if err != nil {
		return err
	}

	return visitVersions(state, func(ver *modelpb.AppengineState_Service_Version) error {
		if ver.CapturedState == nil {
			return errors.Reason("no captured_state field").Err()
		}
		return nil
	})
}

// visitServices calls the callback for each Service proto.
func visitServices(state *modelpb.AppengineState, cb func(*modelpb.AppengineState_Service) error) error {
	if len(state.Services) == 0 {
		return errors.Reason("services list is empty").Err()
	}
	for _, svc := range state.Services {
		if svc.Name == "" {
			return errors.Reason("unnamed service").Err()
		}
		if err := cb(svc); err != nil {
			return errors.Annotate(err, "in service %q", svc.Name).Err()
		}
	}
	return nil
}

// visitVersions calls the callback for each Version proto across all Services.
func visitVersions(state *modelpb.AppengineState, cb func(*modelpb.AppengineState_Service_Version) error) error {
	for _, svc := range state.Services {
		if len(svc.Versions) == 0 {
			return errors.Reason("in service %q: no versions", svc.Name).Err()
		}
		for _, ver := range svc.Versions {
			if ver.Name == "" {
				return errors.Reason("in service %q: unnamed version", svc.Name).Err()
			}
			if err := cb(ver); err != nil {
				return errors.Annotate(err, "in service %q: in version %q", svc.Name, ver.Name).Err()
			}
		}
	}
	return nil
}

func isAppengineUpToDate(intended *modelpb.AppengineState, reported *modelpb.AppengineState) bool {
	// TODO
	return false
}
