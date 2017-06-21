// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

//go:generate stringer -type=Status

package model

import "encoding/json"

// Status is a discrete status for the purpose of colorizing a component.
// These are based off the Material Design Bootstrap color palettes.
type Status int

const (
	// NotRun if the component has not yet been run.
	NotRun Status = iota // 100 Gray

	// Running if the component is currently running.
	Running // 100 Teal

	// Success if the component has finished executing and is not noteworthy.
	Success // A200 Green

	// Failure if the component has finished executing and contains a failure.
	Failure // A200 Red

	// Warning just like from the buildbot days.
	Warning // 200 Yellow

	// InfraFailure if the component has finished incompletely due to a failure in infra.
	InfraFailure // A100 Purple

	// Exception if the component has finished incompletely and unexpectedly. This
	// is used for buildbot builds.
	Exception // A100 Purple

	// Expired if the component was never scheduled due to resource exhaustion.
	Expired // A200 Purple

	// DependencyFailure if the component has finished incompletely due to a failure in a
	// dependency.
	DependencyFailure // 100 Amber

	// WaitingDependency if the component has finished or paused execution due to an
	// incomplete dep.
	WaitingDependency // 100 Brown
)

// Terminal returns true if the step status won't change.
func (s Status) Terminal() bool {
	switch s {
	case Success, Failure, InfraFailure, Warning, DependencyFailure, Expired:
		return true
	default:
		return false
	}
}

// MarshalJSON renders enums into String rather than an int when marshalling.
func (s Status) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}
