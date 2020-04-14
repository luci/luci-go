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

package job

import (
	bbpb "go.chromium.org/luci/buildbucket/proto"
	swarmingpb "go.chromium.org/luci/swarming/proto/api"
)

// Info represents the common low-level information for all job Definitions.
//
// Swarming and Buildbucket implement this interface.
type Info interface {
	// SwarmingHostname retrieves the Swarming hostname this Definition will use.
	SwarmingHostname() string

	// TaskName retrieves the human-readable Swarming task name from the
	// Definition.
	TaskName() string

	// CurrentIsolated returns the current isolated contents for the
	// Definition. If the current CASTree information has no Digest value, this
	// returns nil.
	//
	// Returns error if this is a Swarming job where the slices have differing
	// isolateds.
	CurrentIsolated() (*swarmingpb.CASTree, error)

	// Dimensions returns a single map of all dimensions in the job Definition
	// and what their latest expiration is.
	//
	// For dimensions which "don't expire", they will report an expiration time of
	// the total task expiration.
	Dimensions() (ExpiringDimensions, error)

	// CIPDPkgs returns the mapping of all CIPD packages in the task, in the
	// same format that Editor.CIPDPkgs takes.
	//
	// Returns an error if not all slices in the swarming task have the same set
	// of packages.
	CIPDPkgs() (CIPDPkgs, error)

	// Env returns any environment variable overrides in the job.
	Env() (map[string]string, error)

	// PrefixPathEnv returns the list of $PATH prefixes
	PrefixPathEnv() ([]string, error)

	// Priority returns the job's current swarming priority value.
	Priority() int32

	// Tags returns the job's current swarming tags.
	Tags() []string
}

// HighLevelInfo represents high-level information for Buildbucket job
// Definitions.
//
// HighLevelInfo includes all capabilities of `Info` as well.
type HighLevelInfo interface {
	Info

	// Returns true iff the job is marked as 'experimental'.
	Experimental() bool

	// Returns the input properties of this build as a map of top-level key to
	// its JSON-encoded value.
	Properties() (map[string]string, error)

	// TaskPayload returns information about where the job's payload lives:
	//
	//  * (cipdPkg, cipdVers) - If both are set, they describe the cipd package
	//    where the job will pull the payload from. If unset, payload is assumed
	//    to live in UserPayload.
	//  * pathInTask - the path from the task root to the directory containing the
	//    payload.
	TaskPayload() (cipdPkg, cipdVers string, pathInTask string)

	// Returns the gerrit changes associated with this job.
	GerritChanges() []*bbpb.GerritChange

	// Returns the gitiles commit associated with this job.
	GitilesCommit() *bbpb.GitilesCommit
}

// Info returns a generic accessor object which can return generic information
// about the Definition.
//
// Returns nil if the Definition doesn't support Info().
func (jd *Definition) Info() Info {
	if bb := jd.GetBuildbucket(); bb != nil {
		return bbInfo{bb, jd.UserPayload}
	} else if sw := jd.GetSwarming(); sw != nil {
		return swInfo{sw, jd.UserPayload}
	}
	return nil
}

// HighLevelInfo returns an accessor object which can returns high-level
// information about the Definition.
//
// Only returns a functioning implementation for Buildbucket jobs, otherwise
// returns nil.
func (jd *Definition) HighLevelInfo() HighLevelInfo {
	if bb := jd.GetBuildbucket(); bb != nil {
		return bbInfo{bb, jd.UserPayload}
	}
	return nil
}
