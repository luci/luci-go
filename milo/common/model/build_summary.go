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
	"bytes"
	"time"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/data/cmpbin"
)

// ManifestKey is an index entry for BuildSummary, which looks like
//   0 ++ project ++ console ++ manifest_name ++ url ++ revision.decode('hex')
//
// This is used to index this BuildSummary as the row for any consoles that it
// shows up in that use the Manifest/RepoURL/Revision indexing scheme.
//
// (++ is cmpbin concatenation)
//
// Example:
//   0 ++ "chromium" ++ "main" ++ "UNPATCHED" ++ "https://.../src.git" ++ deadbeef
//
// The list of interested consoles is compiled at build summarization time.
type ManifestKey []byte

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

	// SelfLink provides a relative URL for this build.
	// Buildbot: /buildbot/<mastername>/<buildername>/<buildnumber>
	// Swarmbucket: Derived from Buildbucket (usually link to self)
	SelfLink string

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

	// ManifestKeys is the list of ManifestKey entries for this BuildSummary.
	ManifestKeys []ManifestKey

	// AnnotationURL is the URL to the logdog annotation location. This will be in
	// the form of:
	//   logdog://service.host.example.com/project_id/prefix/+/stream/name
	AnnotationURL string

	// Version can be used by buildsource implementations to compare with an
	// externally provided version number/timestamp to ensure that BuildSummary
	// objects are only updated forwards in time.
	//
	// Known uses:
	//   * Buildbucket populates this with Build.UpdatedTs, which is guaranteed to
	//     be monotonically increasing. Used to ignore out-of-order pubsub
	//     messages.
	Version int64
}

// AddManifestKey adds a new entry to ManifestKey.
//
// `revision` should be the hex-decoded git revision.
//
// It's up to the caller to ensure that entries in ManifestKey aren't
// duplicated.
func (bs *BuildSummary) AddManifestKey(project, console, manifest, repoURL string, revision []byte) {
	bs.ManifestKeys = append(bs.ManifestKeys,
		NewPartialManifestKey(project, console, manifest, repoURL).AddRevision(revision))
}

// PartialManifestKey is an incomplete ManifestKey key which can be made
// complete by calling AddRevision.
type PartialManifestKey []byte

// AddRevision appends a git revision (as bytes) to the PartialManifestKey,
// returning a full index value for BuildSummary.ManifestKey.
func (p PartialManifestKey) AddRevision(revision []byte) ManifestKey {
	var buf bytes.Buffer
	buf.Write(p)
	cmpbin.WriteBytes(&buf, revision)
	return buf.Bytes()
}

// NewPartialManifestKey generates a ManifestKey prefix corresponding to
// the given parameters.
func NewPartialManifestKey(project, console, manifest, repoURL string) PartialManifestKey {
	var buf bytes.Buffer
	cmpbin.WriteUint(&buf, 0) // version
	cmpbin.WriteString(&buf, project)
	cmpbin.WriteString(&buf, console)
	cmpbin.WriteString(&buf, manifest)
	cmpbin.WriteString(&buf, repoURL)
	return PartialManifestKey(buf.Bytes())
}
