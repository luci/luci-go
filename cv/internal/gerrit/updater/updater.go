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

// Package updater fetches latest CL data from Gerrit.
package updater

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap"
)

// UpdateCL fetches latest info from Gerrit.
//
// If datastore already contains snapshot with Gerrit-reported update time equal
// to or after updatedHint, then no updating or querying will be performed.
// To force an update, provide as time.Time{} as updatedHint.
func UpdateCL(ctx context.Context, luciProject, host string, change int64, updatedHint time.Time) (err error) {
	fetcher := fetcher{
		luciProject: luciProject,
		host:        host,
		change:      change,
		updatedHint: updatedHint,
	}
	if fetcher.g, err = gerrit.CurrentClient(ctx, host, luciProject); err != nil {
		return err
	}
	return fetcher.update(ctx)
}

// fetcher efficiently computes new snapshot by fetching data from Gerrit.
//
// It ensures each dependency is resolved to an existing CLID,
// creating CLs in datastore as needed. Schedules tasks to update
// dependencies but doesn't wait for them to complete.
//
// The prior Snapshot, if given, can reduce RPCs made to Gerrit.
type fetcher struct {
	luciProject string
	host        string
	change      int64
	updatedHint time.Time

	g gerrit.CLReaderClient

	externalID changelist.ExternalID
	priorCL    *changelist.CL

	newSnapshot *changelist.Snapshot
	newAcfg     *changelist.ApplicableConfig
}

func (f *fetcher) shouldSkip(ctx context.Context) (skip bool, err error) {
	switch f.priorCL, err = f.externalID.Get(ctx); {
	case err == datastore.ErrNoSuchEntity:
		return false, nil
	case err != nil:
		return false, err
	case f.priorCL.Snapshot == nil:
		// CL is likely created as a dependency and not yet populated.
		return false, nil

	case f.priorCL.Snapshot.GetGerrit().GetInfo() == nil:
		panic(errors.Reason("%s has snapshot without Gerrit Info", f).Err())
	case f.priorCL.ApplicableConfig == nil:
		panic(errors.Reason("%s has snapshot but not ApplicableConfig", f).Err())

	case !f.updatedHint.IsZero() && f.priorCL.Snapshot.IsUpToDate(f.luciProject, f.updatedHint):
		ci := f.priorCL.Snapshot.GetGerrit().Info
		switch acfg, err := gobmap.Lookup(ctx, f.host, ci.GetProject(), ci.GetRef()); {
		case err != nil:
			return false, err
		case acfg.HasProject(f.luciProject):
			logging.Debugf(ctx, "Updating %s to %s skipped, already at %s", f, f.updatedHint,
				f.priorCL.Snapshot.GetExternalUpdateTime().AsTime())
			return true, nil
		default:
			// CL is no longer watched by the given luciProject, even though
			// snapshot is considered up-to-date.
			return true, changelist.Update(ctx, "", f.priorCL.ID, nil /*keep snapshot as is*/, acfg)
		}
	}
	return false, nil
}

func (f *fetcher) update(ctx context.Context) (err error) {
	f.externalID, err = changelist.GobID(f.host, f.change)
	if err != nil {
		return err
	}

	switch skip, err := f.shouldSkip(ctx); {
	case err != nil:
		return err
	case skip:
		return nil
	}

	f.newSnapshot = &changelist.Snapshot{Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{}}}
	// TODO(tandrii): optimize for existing CL case.
	if err := f.new(ctx); err != nil {
		return err
	}

	min, cur, err := gerrit.EquivalentPatchsetRange(f.newSnapshot.GetGerrit().GetInfo())
	if err != nil {
		return err
	}
	f.newSnapshot.MinEquivalentPatchset = int32(min)
	f.newSnapshot.Patchset = int32(cur)
	return changelist.Update(ctx, f.externalID, f.clidIfKnown(), f.newSnapshot, f.newAcfg)
}

// new efficiently fetches new snapshot from Gerrit.
func (f *fetcher) new(ctx context.Context) error {
	req := &gerritpb.GetChangeRequest{
		Number:  f.change,
		Project: f.gerritProjectIfKnown(),
		Options: []gerritpb.QueryOption{
			// These are expensive to compute for Gerrit,
			// CV should not do this needlessly.
			gerritpb.QueryOption_ALL_REVISIONS,
			gerritpb.QueryOption_CURRENT_COMMIT,
			gerritpb.QueryOption_DETAILED_LABELS,
			gerritpb.QueryOption_DETAILED_ACCOUNTS,
			gerritpb.QueryOption_MESSAGES,
			gerritpb.QueryOption_SUBMITTABLE,
			// Avoid asking Gerrit to perform expensive operation.
			gerritpb.QueryOption_SKIP_MERGEABLE,
		},
	}
	ci, err := f.g.GetChange(ctx, req)
	switch grpcutil.Code(err) {
	case codes.OK:
		if err := f.ensureNotStale(ctx, ci.GetUpdated()); err != nil {
			return err
		}
		f.newSnapshot.GetGerrit().Info = ci
		f.newSnapshot.ExternalUpdateTime = ci.GetUpdated()
	case codes.NotFound:
		// Either no access OR CL was deleted.
		return errors.New("not implemented")
	case codes.PermissionDenied:
		return errors.New("not implemented")
	default:
		return unhandledError(ctx, err, "failed to fetch %s", f)
	}

	switch ci.GetStatus() {
	case gerritpb.ChangeStatus_NEW:
		// OK, proceed.
	case gerritpb.ChangeStatus_ABANDONED, gerritpb.ChangeStatus_MERGED:
		logging.Debugf(ctx, "%s is %s", f, ci.GetStatus())
		return nil
	default:
		logging.Warningf(ctx, "%s has unknown status %d %s", f, ci.GetStatus().Number(), ci.GetStatus().String())
		return nil
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error { return f.fetchFiles(ctx) })
	eg.Go(func() error { return f.fetchRelated(ctx) })
	if err = eg.Wait(); err != nil {
		return err
	}
	return errors.New("not implemented")
}

// fetchRelated fetches related changes and computes GerritGitDeps.
func (f *fetcher) fetchRelated(ctx context.Context) error {
	resp, err := f.g.GetRelatedChanges(ctx, &gerritpb.GetRelatedChangesRequest{
		Number:     f.change,
		Project:    f.gerritProjectIfKnown(),
		RevisionId: f.mustHaveCurrentRevision(),
	})
	switch code := grpcutil.Code(err); code {
	case codes.OK:
		return f.setGitDeps(ctx, resp.GetChanges())
	case codes.PermissionDenied, codes.NotFound:
		// Getting this right after successfully fetching ChangeInfo should
		// typically be due to eventual consistency of Gerrit, and rarely due to
		// change of ACLs. So, err transiently s.t. retry handles the same error
		// when re-fetching ChangeInfo.
		return errors.Annotate(err, "failed to fetch related changes for %s", f).Tag(transient.Tag).Err()
	default:
		return unhandledError(ctx, err, "failed to fetch related changes for %s", f)
	}
}

// setGitDeps sets GerritGitDeps based on list of related changes provided by
// Gerrit.GetRelatedChanges RPC.
func (f *fetcher) setGitDeps(ctx context.Context, related []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit) error {
	// Gerrit does not provide API that returns just the changes which a given
	// change depends on, but has the API call that returns the following changes:
	//   (1) those on which this change depends, transitively. Among these,
	//       some CLs may have been already merged.
	//   (2) this change itself, with its commit and parent(s) hashes
	//   (3) changes which depend on this change transitively
	// We need (1).
	if len(related) == 0 {
		// Gerrit may not bother to return the CL itself if there are no related
		// changes.
		return nil
	}
	this, err := f.matchCurrentAmongRelated(ctx, related)
	if err != nil {
		return err
	}

	// Construct a map from revision to a list of changes that it represents.
	// One may think that list is not necessary:
	//   two CLs with the same revision should (% sha1 collision) have equal
	//   commit messages, and hence Change-Id, so should be really the same CL.
	// However, many Gerrit projects do not require Change-Id in commit message at
	// upload time, instead generating new Change-Id on the fly.
	byRevision := make(map[string][]*gerritpb.GetRelatedChangesResponse_ChangeAndCommit, len(related))
	for _, r := range related {
		rev := r.GetCommit().GetId()
		byRevision[rev] = append(byRevision[rev], r)
	}

	thisParentsCount := f.countRelatedWhichAreParents(this, byRevision)
	if thisParentsCount == 0 {
		// Quick exit if there are no dependencies of this change (1), only changes
		// depending on this change (3).
		return nil
	}

	// Now starting from `this` change and following parents relation,
	// find all issues that we can reach via breadth first traversal ordeded by
	// distance from this CL.
	// Note that diamond-shaped child->[parent1, parent2]->grantparent are
	// probably possible, so keeping track of visited commits is required.
	// Furthermore, the same CL number may appear multiple times in the chain
	// under different revisions (patchsets).
	visitedRevs := stringset.New(len(related))
	ordered := make([]*gerritpb.GetRelatedChangesResponse_ChangeAndCommit, 0, len(related))
	curLevel := make([]*gerritpb.GetRelatedChangesResponse_ChangeAndCommit, 0, len(related))
	nextLevel := make([]*gerritpb.GetRelatedChangesResponse_ChangeAndCommit, 1, len(related))
	nextLevel[0] = this
	for len(nextLevel) > 0 {
		curLevel, nextLevel = nextLevel, curLevel[:0]
		// For determinism of the output.
		sort.SliceStable(curLevel, func(i, j int) bool {
			return curLevel[i].GetNumber() < curLevel[j].GetNumber()
		})
		ordered = append(ordered, curLevel...)
		for _, r := range curLevel {
			for _, p := range r.GetCommit().GetParents() {
				switch prs := byRevision[p.GetId()]; {
				case len(prs) == 0:
					continue
				case len(prs) > 1:
					logging.Warningf(ctx, "Gerrit.GetRelatedChanges returned rev %q %d times for %s"+
						"(ALL Related %s)", p.GetId(), len(prs), f, related)
					// Avoid borking. Take the first CL by number.
					for i, x := range prs[1:] {
						if prs[0].GetNumber() > x.GetNumber() {
							prs[i+1], prs[0] = prs[0], prs[i+1]
						}
					}
					fallthrough
				default:
					if visitedRevs.Add(prs[0].GetCommit().GetId()) {
						nextLevel = append(nextLevel, prs[0])
					}
				}
			}
		}
	}

	deps := make([]*changelist.GerritGitDep, 0, len(ordered)-1)
	// Specific revision doesn't matter, CV always looks at latest revision,
	// but since the same CL may have >1 revision, the CL number may be added
	// several times into `ordered`.
	// TODO(tandrii): after CQDaemon is removed, consider paying attention to
	// specific revision of the dependency to notice when parent dep has been
	// substantially modified such that tryjobs of this change alone ought to be
	// invalidated (see https://crbug.com/686115).
	added := make(map[int64]bool, len(ordered))
	for i, r := range ordered[1:] {
		n := r.GetNumber()
		if added[n] {
			continue
		}
		added[n] = true
		deps = append(deps, &changelist.GerritGitDep{
			Change: n,
			// By construction of ordered, immediate dependencies must be located at
			// ordered[1:1+thisParentsCount], but we are iterating over [1:] subslice.
			Immediate: i < thisParentsCount,
		})
	}
	f.newSnapshot.GetGerrit().GitDeps = deps
	return nil
}

func (f *fetcher) matchCurrentAmongRelated(ctx context.Context, related []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit) (
	this *gerritpb.GetRelatedChangesResponse_ChangeAndCommit, err error) {
	matched := 0
	for _, r := range related {
		if r.GetNumber() == f.change {
			matched++
			this = r
		}
	}
	// Sanity check for better error message if Gerrit API changes.
	if matched != 1 {
		logging.Errorf(ctx, "Gerrit.GetRelatedChanges should have exactly 1 change "+
			"matching the given one %s, got %d (all related %s)", f, matched, related)
		return nil, errors.Reason("Unexpected Gerrit.GetRelatedChangesResponse for %s, see logs", f).Err()
	}
	return this, nil
}

func (f *fetcher) countRelatedWhichAreParents(this *gerritpb.GetRelatedChangesResponse_ChangeAndCommit, byRevision map[string][]*gerritpb.GetRelatedChangesResponse_ChangeAndCommit) int {
	cnt := 0
	for _, p := range this.GetCommit().GetParents() {
		// Not all parents may be represented by related CLs.
		// OTOH, if there are several CLs matching parent revision,
		// CV will choose just one.
		if _, ok := byRevision[p.GetId()]; ok {
			cnt++
		}
	}
	return cnt
}

// fetchFiles fetches files for the current revision of the new Snapshot.
func (f *fetcher) fetchFiles(ctx context.Context) error {
	resp, err := f.g.ListFiles(ctx, &gerritpb.ListFilesRequest{
		Number:     f.change,
		Project:    f.gerritProjectIfKnown(),
		RevisionId: f.mustHaveCurrentRevision(),
		// For CLs with >1 parent commit (aka merge commits), this relies on Gerrit
		// ensuring that such a CL always has first parent from the target branch.
		Parent: 1, // Request a diff against the first parent.
	})
	switch code := grpcutil.Code(err); code {
	case codes.OK:
		// Iterate files map and take keys only. CV treats all files "touched" in a
		// Change to be interesting, including chmods. Skip special /COMMIT_MSG and
		// /MERGE_LIST entries, which aren't files. For example output, see
		// https://chromium-review.googlesource.com/changes/1817639/revisions/1/files?parent=1
		fs := make([]string, 0, len(resp.GetFiles()))
		for f := range resp.GetFiles() {
			if !strings.HasPrefix(f, "/") {
				fs = append(fs, f)
			}
		}
		sort.Strings(fs)
		f.newSnapshot.GetGerrit().Files = fs
		return nil

	case codes.PermissionDenied, codes.NotFound:
		// Getting this right after successfully fetching ChangeInfo should
		// typically be due to eventual consistency of Gerrit, and rarely due to
		// change of ACLs. So, err transiently s.t. retry handles the same error
		// when re-fetching ChangeInfo.
		return errors.Annotate(err, "failed to fetch files for %s", f).Tag(transient.Tag).Err()

	default:
		return unhandledError(ctx, err, "failed to fetch files for %s", f)
	}
}

// ensureNotStale returns error if given Gerrit updated timestamp is older than
// the updateHint or existing CL state.
func (f *fetcher) ensureNotStale(ctx context.Context, externalUpdateTime *timestamppb.Timestamp) error {
	t := externalUpdateTime.AsTime()
	storedTS := f.priorSnapshot().GetExternalUpdateTime()

	switch {
	case !f.updatedHint.IsZero() && f.updatedHint.After(t):
		logging.Errorf(ctx, "Fetched last Gerrit update of %s, but %s expected", t, f.updatedHint)
	case storedTS != nil && storedTS.AsTime().Before(t):
		logging.Errorf(ctx, "Fetched last Gerrit update of %s, but %s was already seen & stored", t, storedTS.AsTime())
	default:
		return nil
	}
	return errors.Reason("Fetched stale Gerrit data").Tag(transient.Tag).Err()
}

// unhandledError is used to process and annotate Gerrit errors.
func unhandledError(ctx context.Context, err error, format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	ann := errors.Annotate(err, msg)
	switch code := grpcutil.Code(err); code {
	case
		codes.OK,
		codes.PermissionDenied,
		codes.NotFound,
		codes.FailedPrecondition:
		// These must be handled before.
		logging.Errorf(ctx, "FIXME unhandled Gerrit error: %s while %s", err, msg)
		return ann.Err()

	case
		codes.InvalidArgument,
		codes.Unauthenticated:
		// This must not happen in practice unless there is a bug in CV or Gerrit.
		logging.Errorf(ctx, "FIXME bug in CV: %s while %s", err, msg)
		return ann.Err()

	case codes.Unimplemented:
		// This shouldn't happen in production, but may happen in development
		// if gerrit.NewRESTClient doesn't actually implement fully the option
		// or entire method that CV is coded to work with.
		logging.Errorf(ctx, "FIXME likely bug in CV: %s while %s", err, msg)
		return ann.Err()

	default:
		// Assume transient. If this turns out non-transient, then its code must be
		// handled explicitly above.
		return ann.Tag(transient.Tag).Err()
	}
}

func (f *fetcher) gerritProjectIfKnown() string {
	if project := f.priorSnapshot().GetGerrit().GetInfo().GetProject(); project != "" {
		return project
	}
	if project := f.newSnapshot.GetGerrit().GetInfo().GetProject(); project != "" {
		return project
	}
	return ""
}

func (f *fetcher) clidIfKnown() changelist.CLID {
	if f.priorCL != nil {
		return f.priorCL.ID
	}
	return 0
}

func (f *fetcher) priorSnapshot() *changelist.Snapshot {
	if f.priorCL != nil {
		return f.priorCL.Snapshot
	}
	return nil
}

func (f *fetcher) priorAcfg() *changelist.ApplicableConfig {
	if f.priorCL != nil {
		return f.priorCL.ApplicableConfig
	}
	return nil
}

func (f *fetcher) mustHaveCurrentRevision() string {
	switch ci := f.newSnapshot.GetGerrit().GetInfo(); {
	case ci == nil:
		panic("ChangeInfo must be already fetched into newSnapshot")
	case ci.GetCurrentRevision() == "":
		panic("ChangeInfo must have CurrentRevision populated.")
	default:
		return ci.GetCurrentRevision()
	}
}

// String is used for debug identification of a fetch in errors and logs.
func (f *fetcher) String() string {
	if f.priorCL == nil {
		return fmt.Sprintf("CL(%s/%d)", f.host, f.change)
	}
	return fmt.Sprintf("CL(%s/%d [%d])", f.host, f.change, f.priorCL.ID)
}
