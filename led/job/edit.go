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
	"go.chromium.org/luci/common/errors"
	swarmingpb "go.chromium.org/luci/swarming/proto/api"
)

// Editor represents low-level mutations you can make on a job.Definition.
//
// You may edit a job.Definition by calling its Edit method.
type Editor interface {
	// ClearCurrentIsolated removes all isolateds from this Definition.
	ClearCurrentIsolated()

	// ClearDimensions removes all dimensions.
	ClearDimensions()

	// SetDimensions sets the full set of dimensions.
	SetDimensions(dims ExpiringDimensions)

	// EditDimensions edits the swarming dimensions.
	EditDimensions(dimEdits DimensionEditCommands)

	// CIPDPkgs allows you to edit the cipd packages. The mapping is in the form
	// of:
	//
	//    "subdir:name/of/package" -> "version"
	//
	// If version is empty, this package will be removed (if it's present).
	CIPDPkgs(cipdPkgs CIPDPkgs)

	// Env edits the swarming environment variables (i.e. those set before the
	// user payload runs).
	//
	// The map given is a map of environment variable to its value; If the value
	// is "", then the environment variable is removed.
	Env(env map[string]string)

	// PrefixPathEnv controls swarming's env_prefix mapping for $PATH.
	//
	// Values prepended with '!' will remove them from the existing list of values
	// (if present). Otherwise these values will be appended to the current list
	// of PATH-prefix-envs.
	PrefixPathEnv(values []string)

	// Priority edits the swarming task priority.
	Priority(priority int32)

	// SwarmingHostname allows you to modify the current SwarmingHostname used by
	// this led pipeline.
	SwarmingHostname(host string)

	// TaskName allows you to set the swarming task name of the job.
	TaskName(name string)

	// Tags appends the given values to the task's tags.
	Tags(values []string)
}

// HighLevelEditor represents high-level mutations you can make on
// a job.Definition.
//
// Currently only Buildbucket job.Definitions support high level edits.
//
// You may edit a job.Definition by calling its HighLevelEdit method.
type HighLevelEditor interface {
	Editor

	// Experimental sets the task to either be marked 'experimental' or not.
	Experimental(isExperimental bool)

	// Experiments enables or disables experiments.
	//
	// If `exps[<name>]` is true, the corresponding experiment will be enabled.
	// If it is false, the experiment will be disabled (if it was enabled).
	//
	// Experiments enabled in the build, but not mentioned in `exps` map are left
	// enabled.
	Experiments(exps map[string]bool)

	// Properties edits the recipe properties.
	//
	// `props` should be a mapping of the top-level property to JSON-encoded data
	// for the value of this property. If the value is "", then the top-level
	// property will be deleted.
	//
	// If `auto` is true, then values which do not decode as JSON will be treated
	// as literal strings. For example, given a value `literal string` with
	// auto=true, this would be equivalent to passing the value `"literal string"`
	// with auto=false.
	Properties(props map[string]string, auto bool)

	// TaskPayloadSource sets the task payload to either use the provided CIPD
	// package and version, or the UserPayload if they are empty.
	//
	// If cipdPkg is specified without cipdVers, cipdVers will be set to "latest".
	TaskPayloadSource(cipdPkg, cipdVers string)

	// TaskPayloadPath sets the location of where bbagent should locate the task
	// payload data within the task.
	TaskPayloadPath(path string)

	// CASTaskPayload sets the cas reference as the build's user payload.
	CASTaskPayload(path string, casRef *swarmingpb.CASReference)

	// TaskPayloadCmd sets the arguments used to run the task payload.
	//
	// args[0] is the path, relative to the payload path, of the executable to
	// run.
	//
	// NOTE: Kitchen tasks ignore this value.
	TaskPayloadCmd(args []string)

	// ClearGerritChanges removes all GerritChanges from the job.
	ClearGerritChanges()

	// AddGerritChange ensures the GerritChange is in the set of input CLs.
	AddGerritChange(cl *bbpb.GerritChange)

	// RemoveGerritChange ensures the GerritChange is not in the set of input CLs.
	RemoveGerritChange(cl *bbpb.GerritChange)

	// GitilesCommit sets the GitilesCommit.
	//
	// If `commit` is nil, removes the commit.
	GitilesCommit(commit *bbpb.GitilesCommit)
}

// Edit runs a mutator function with an Editor.
//
// If one of the edit operations has an error, further method calls on the
// Editor are ignored and this returns the error.
func (jd *Definition) Edit(cb func(m Editor)) error {
	var m interface {
		Editor
		Close() error
	}

	if jd.GetBuildbucket() != nil {
		m = newBuildbucketEditor(jd)
	} else {
		m = newSwarmingEditor(jd)
	}

	cb(m)
	return m.Close()
}

// HighLevelEdit runs a mutator function with a HighLevelEditor.
//
// If one of the edit operations has an error, further method calls on the
// HighLevelEditor are ignored and this returns the error.
func (jd *Definition) HighLevelEdit(cb func(m HighLevelEditor)) error {
	var m interface {
		HighLevelEditor
		Close() error
	}

	if jd.GetBuildbucket() != nil {
		m = newBuildbucketEditor(jd)
	} else {
		return errors.New("job.HighLevelEdit not supported for this Job")
	}

	cb(m)
	return m.Close()
}
