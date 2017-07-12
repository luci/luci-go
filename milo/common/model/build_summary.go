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

package model

import (
	"time"

	"github.com/luci/gae/service/datastore"
)

// BuildSummary is a datastore model which is used for storing staandardized
// summarized build data, and is used for backend-agnostic views (i.e. builders,
// console). It contains only data that:
//   * is necessary to render these simplified views
//   * is present in all implementations (buildbot, buildbucket)
//
// This entity will live as a child of the various implementation's
// representations of a build (e.g. buildbotBuild). It has various 'tag' fields
// so that it can be queried by the various backend-agnostic views.
type BuildSummary struct {
	// _id for a BuildSummary is always 1
	_ int64 `gae:"$id,1"`

	// BuildKey will always point to the "real" build, i.e. a buildbotBuild or
	// a buildbucketBuild. It is always the parent key for the BuildSummary.
	BuildKey *datastore.Key `gae:"$parent"`

	// Global identifier for the builder that this Build belongs to, i.e.:
	//   "buildbot/<mastername>/<buildername>"
	//   "buildbucket/<bucketname>/<buildername>"
	BuilderID string

	// KnownConsoleHash is used for backfilling and must always equal the raw
	// value of:
	//
	//   sha256(sorted(ConsoleEpochs.keys())
	//
	// This is used to identify BuildSummaries which haven't yet been included in
	// some new Console definition (or which have been removed from a Console
	// definition).
	KnownConsoleHash []byte

	// ConsoleEpochs is used for backfilling, and is a series of cmpbin tuples:
	//
	//   (console_name[str], recorded_epoch[int])
	//
	// This maps which epoch (version) of a console definition this BuildSummary
	// belongs to. Whenever a console definition changes, its epoch increases. The
	// backfiller will then identify BuildSummary objects which are out of date
	// and update them.
	ConsoleEpochs [][]byte

	// ConsoleTags contains query tags for the console view. These are cmpbin
	// tuples which look like:
	//
	//   (console_name[str], sort_criteria[tuple], sort_values[tuple])
	//
	// `sort_criteria` are defined by the console definition, and will likely look
	// like (manifest_name[str], repo_url[str]), but other sort_criteria may be
	// added later.
	//
	// `sort_values` are defined by the selected sort_criteria, and will likely
	// look like (commit_revision[str],). In any event, this string is opaque and
	// only to be used by the console itself.
	ConsoleTags [][]byte

	// Created is the time when the Build was first created. Due to pending
	// queues, this may be substantially before Summary.Start.
	Created time.Time

	// Summary summarizes relevant bits about the overall build.
	Summary Summary

	// CurrentStep summarizes relevant bits about the currently running step (if
	// any). Only expected to be set if !Summary.Status.Terminal().
	CurrentStep Summary

	// Manifests is a list of links to source manifests that this build reported.
	Manifests []ManifestLink

	// Patches is the list of patches which are associated with this build.
	// We reserve the multi-patch case for advanced (multi-repo) tryjobs...
	// Typically there will only be one patch associated with a build.
	Patches []PatchInfo
}
