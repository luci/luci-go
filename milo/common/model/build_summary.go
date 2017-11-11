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
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/data/cmpbin"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	milo "go.chromium.org/luci/milo/api/proto"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/git"
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

const currentManifestKeyVersion = 0

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

	// Global identifier for this Build.
	// Buildbot: "buildbot/<mastername>/<buildername>/<buildnumber>"
	// Buildbucket: "buildbucket/<buildaddr>"
	// For buildbucket, <buildaddr> looks like <bucketname>/<buildername>/<buildnumber> if available
	// and <buildid> otherwise.
	BuildID string

	// The LUCI project ID associated with this build. This is used for ACL checks
	// when presenting this build to end users.
	ProjectID string

	// This contains URI to any contextually-relevant underlying tasks/systems
	// associated with this build, in the form of:
	//
	//   * swarming://<host>/task/<taskID>
	//   * swarming://<host>/bot/<botID>
	//   * buildbot://<master>/build/<builder>/<number>
	//   * buildbot://<master>/bot/<bot>
	//
	// This will be used for queries, and can be used to store semantically-sound
	// clues about this Build (e.g. to identify the underlying swarming task so
	// that we don't need to RPC back to the build source to find that out). This
	// can also be used for link generation in the UI, since the schema for these
	// URIs should be stable within Milo (so if swarming changes its URL format we
	// can change the links in the UI code without map-reducing these
	// ContextURIs).
	ContextURI []string

	// The buildbucket buildsets associated with this Build, if any.
	//
	// Example:
	//   commit/gitiles/<host>/<project/path>/+/<commit>
	//
	// See https://chromium.googlesource.com/infra/infra/+/master/appengine/cr-buildbucket/doc/index.md#buildset-tag
	BuildSet []string

	// SelfLink provides a relative URL for this build.
	// Buildbot: /buildbot/<mastername>/<buildername>/<buildnumber>
	// Swarmbucket: Derived from Buildbucket (usually link to self)
	SelfLink string

	// Created is the time when the Build was first created. Due to pending
	// queues, this may be substantially before Summary.Start.
	Created time.Time

	// Summary summarizes relevant bits about the overall build.
	Summary Summary

	// Manifests is a list of links to source manifests that this build reported.
	Manifests []ManifestLink

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

	// Ignore all extra fields when reading/writing
	_ datastore.PropertyMap `gae:"-,extra"`
}

// GetConsoleNames extracts the "<project>/<console>" names from the
// BuildSummary's current ManifestKeys.
func (bs *BuildSummary) GetConsoleNames() ([]string, error) {
	ret := stringset.New(0)
	for _, mk := range bs.ManifestKeys {
		r := bytes.NewReader(mk)
		vers, _, err := cmpbin.ReadUint(r)
		if err != nil || vers != currentManifestKeyVersion {
			continue
		}
		proj, _, err := cmpbin.ReadString(r)
		if err != nil {
			return nil, errors.Annotate(err, "couldn't parse proj").Err()
		}
		console, _, err := cmpbin.ReadString(r)
		if err != nil {
			return nil, errors.Annotate(err, "couldn't parse console").Err()
		}
		ret.Add(fmt.Sprintf("%s/%s", proj, console))
	}
	retSlice := ret.ToSlice()
	sort.Strings(retSlice)
	return retSlice, nil
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
	cmpbin.WriteUint(&buf, currentManifestKeyVersion) // version
	cmpbin.WriteString(&buf, project)
	cmpbin.WriteString(&buf, console)
	cmpbin.WriteString(&buf, manifest)
	cmpbin.WriteString(&buf, repoURL)
	return PartialManifestKey(buf.Bytes())
}

// AddManifestKeysFromBuildSets potentially adds one or more ManifestKey's to
// the BuildSummary for any defined BuildSets..
//
// This assumes that bs.BuilderID and bs.BuildSet have already been populated.
// If BuilderID is not populated, this will return an error.
func (bs *BuildSummary) AddManifestKeysFromBuildSets(c context.Context) error {
	if bs.BuilderID == "" {
		return errors.New("BuilderID is empty")
	}

	for _, bsetRaw := range bs.BuildSet {
		commit, ok := buildbucket.ParseBuildSet(bsetRaw).(*buildbucket.GitilesCommit)
		if !ok {
			continue
		}

		revision, err := hex.DecodeString(commit.Revision)
		switch {
		case err != nil:
			logging.WithError(err).Warningf(c, "failed to decode revision: %v", commit.Revision)

		case len(revision) != sha1.Size:
			logging.Warningf(c, "wrong revision size %d v %d: %q", len(revision), sha1.Size, commit.Revision)

		default:
			consoles, err := common.GetAllConsoles(c, bs.BuilderID)
			if err != nil {
				return err
			}

			// HACK(iannucci): Until we have real manifest support, console definitions
			// will specify their manifest as "REVISION", and we'll do lookups with null
			// URL fields.
			for _, con := range consoles {
				bs.AddManifestKey(con.GetProjectID(), con.ID, "REVISION", "", revision)

				bs.AddManifestKey(con.GetProjectID(), con.ID, "BUILD_SET/GitilesCommit",
					commit.RepoURL(), revision)
			}
		}
	}
	return nil
}

// GitilesCommit extracts the first BuildSet which is a valid GitilesCommit.
//
// If no such BuildSet is found, this returns nil.
func (bs *BuildSummary) GitilesCommit() *buildbucket.GitilesCommit {
	for _, bset := range bs.BuildSet {
		if gc, ok := buildbucket.ParseBuildSet(bset).(*buildbucket.GitilesCommit); ok {
			return gc
		}
	}
	return nil
}

// ErrUnknownPreviousBuild is returned when PreviousByGitilesCommit was unable
// to find the previous build.
var ErrUnknownPreviousBuild = errors.New("unable to find previous build")

// PreviousByGitilesCommit returns the previous build(s) (and all invervening
// commits) based on the GitilesCommit BuildSet of the given build.
//
// There may be multiple BuildSummaries, if there were rebuilds. The resulting
// BuildSummaries will be sorted in reverse creation order, so that builds[0] is
// always the most-recently-created build.
//
// This will only look up to 100 commits into the past.
//
// If this is unable to find the previous build, it returns
// ErrUnknownPreviousBuild.
func (bs *BuildSummary) PreviousByGitilesCommit(c context.Context) (builds []*BuildSummary, hist *milo.ConsoleGitInfo, err error) {
	gc := bs.GitilesCommit()
	if gc == nil {
		err = ErrUnknownPreviousBuild
		return
	}

	// TODO(iannucci): this could be done in some sort of search pattern, e.g.
	// 10+20+50, but I'm not sure if it's worth it.
	hist, err = git.GetHistory(c, gc.RepoURL(), gc.Revision, 100)
	if err != nil {
		return
	}

	// TODO(iannucci): This bit could be parallelized, but I think in the typical
	// case this will be fast enough.
	curGC := &buildbucket.GitilesCommit{Host: gc.Host, Project: gc.Project}
	q := datastore.NewQuery("BuildSummary").Eq("BuilderID", bs.BuilderID)
	for i, commit := range hist.Commits {
		curGC.Revision = hex.EncodeToString(commit.Hash)
		if err = datastore.GetAll(c, q.Eq("BuildSet", curGC.String()), &builds); err != nil {
			return
		}
		if len(builds) > 0 {
			sort.Slice(builds, func(i, j int) bool {
				return builds[i].Summary.Start.After(builds[j].Summary.Start)
			})
			hist.Commits = hist.Commits[:i+1]
			return
		}
	}

	err = ErrUnknownPreviousBuild
	return
}
