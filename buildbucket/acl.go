// Copyright 2017 The LUCI Authors.
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

package buildbucket

import (
	"fmt"
)

type Action int

const (
	// AddBuild: Schedule a build.
	AddBuild Action = iota

	// ViewBuild: Get information about a build.
	ViewBuild Action = iota

	// LeaseBuild: Lease a build for execution. Normally done by build systems.
	LeaseBuild Action = iota

	// CancelBuild: Cancel an existing build. Does not require a lease key.
	CancelBuild Action = iota

	// ResetBuild: Unlease and reset state of an existing build. Normally done by admins.
	ResetBuild Action = iota

	// SearchBuilds: Search for builds or get a list of scheduled builds.
	SearchBuilds Action = iota

	// ReadACL: View bucket ACLs.
	ReadACL Action = iota

	// WriteACL: Change bucket ACLs.
	WriteACL Action = iota

	// DeleteScheduledBuilds: Delete all scheduled builds from a bucket.
	DeleteScheduledBuilds Action = iota

	// AccessBucket: Know about bucket existence and read its info.
	AccessBucket Action = iota

	// PauseBucket: Pause builds for a given bucket.
	PauseBucket Action = iota

	// SetNextNumber: Set the number for the next build in a builder.
	SetNextNumber Action = iota
)

// ParseAction parses the action name into an Action.
func ParseAction(action string) (Action, error) {
	switch action {
	case "ADD_BUILD":
		return AddBuild, nil
	case "VIEW_BUILD":
		return ViewBuild, nil
	case "LEASE_BUILD":
		return LeaseBuild, nil
	case "CANCEL_BUILD":
		return CancelBuild, nil
	case "RESET_BUILD":
		return ResetBuild, nil
	case "SEARCH_BUILDS":
		return SearchBuilds, nil
	case "READ_ACL":
		return ReadACL, nil
	case "WRITE_ACL":
		return WriteACL, nil
	case "DELETE_SCHEDULED_BUILDS":
		return DeleteScheduledBuilds, nil
	case "ACCESS_BUCKET":
		return AccessBucket, nil
	case "PAUSE_BUCKET":
		return PauseBucket, nil
	case "SET_NEXT_NUMBER":
		return SetNextNumber, nil
	default:
		return -1, fmt.Errorf("unexpected action %q", action)
	}
}

// String returns the action name as a string.
func (a Action) String() string {
	switch a {
	case AddBuild:
		return "ADD_BUILD"
	case ViewBuild:
		return "VIEW_BUILD"
	case LeaseBuild:
		return "LEASE_BUILD"
	case CancelBuild:
		return "CANCEL_BUILD"
	case ResetBuild:
		return "RESET_BUILD"
	case SearchBuilds:
		return "SEARCH_BUILDS"
	case ReadACL:
		return "READ_ACL"
	case WriteACL:
		return "WRITE_ACL"
	case DeleteScheduledBuilds:
		return "DELETE_SCHEDULED_BUILDS"
	case AccessBucket:
		return "ACCESS_BUCKET"
	case PauseBucket:
		return "PAUSE_BUCKET"
	case SetNextNumber:
		return "SET_NEXT_NUMBER"
	default:
		return fmt.Sprintf("unexpected action %d", a)
	}
}
