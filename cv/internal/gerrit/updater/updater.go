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
	"strconv"
	"strings"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/tq"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/cqdepend"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap"
)

func init() {
	tq.RegisterTaskClass(tq.TaskClass{
		ID:        "refresh-gerrit-cl",
		Prototype: &RefreshGerritCL{},
		Queue:     "refresh-gerrit-cl",
		Handler: func(ctx context.Context, payload proto.Message) error {
			t := payload.(*RefreshGerritCL)

			var updatedHint time.Time
			if u := t.GetUpdatedHint(); u != nil {
				updatedHint = u.AsTime()
			}

			return refreshExternal(ctx, t.GetLuciProject(), t.GetHost(), t.GetChange(),
				updatedHint, changelist.CLID(t.GetClidHint()))
		},
	})
}

// Schedule enqueues a TQ task to refresh a Gerrit CL.
func Schedule(ctx context.Context, luciProject, host string, change int64,
	updatedHint time.Time, clidHint changelist.CLID) error {
	payload := &RefreshGerritCL{
		LuciProject: luciProject,
		Host:        host,
		Change:      change,
	}
	task := &tq.Task{
		Payload: payload,
		Title:   fmt.Sprintf("refresh-%s-%d", host, change),
	}

	if clidHint != 0 {
		task.Title += fmt.Sprintf("[%d]", clidHint)
		payload.ClidHint = int64(clidHint)
	}
	if !updatedHint.IsZero() {
		payload.UpdatedHint = timestamppb.New(updatedHint)
		task.Title += fmt.Sprintf("@%s", updatedHint)
	}

	// If done within transaction, can't use de-dup.
	if datastore.CurrentTransaction(ctx) == nil {
		ts := updatedHint
		if updatedHint.IsZero() {
			ts = clock.Now(ctx).Truncate(blindRefreshInterval).Add(blindRefreshInterval)
		}
		task.DeduplicationKey = strings.Join([]string{
			"v0",
			luciProject,
			host, strconv.FormatInt(change, 16),
			strconv.FormatInt(ts.Unix(), 16)},
			"\n")
	}
	return tq.AddTask(ctx, task)
}

// blindRefreshInterval sets interval between blind refresh of a Gerrit CL.
//
// Doesn't affect refreshes with updatedHint specified.
const blindRefreshInterval = time.Minute

// refreshExternal fetches latest info from Gerrit.
//
// If datastore already contains snapshot with Gerrit-reported update time equal
// to or after updatedHint, then no updating or querying will be performed.
// To force an update, provide as time.Time{} as updatedHint.
func refreshExternal(ctx context.Context, luciProject, host string, change int64, updatedHint time.Time, clidHint changelist.CLID) (err error) {
	fetcher := fetcher{
		luciProject: luciProject,
		host:        host,
		change:      change,
		updatedHint: updatedHint,
		// TODO(tandrii): take advantage of clidHint when getting existing CL from
		// datastore.
	}
	fetcher.externalID, err = changelist.GobID(host, change)
	if err != nil {
		return err
	}
	switch fetcher.priorCL, err = fetcher.externalID.Get(ctx); {
	case err == datastore.ErrNoSuchEntity:
		// proceed straight to update.
	case err != nil:
		return err
	default:
		switch skip, err := fetcher.shouldSkip(ctx); {
		case err != nil:
			return err
		case skip:
			return nil
		}
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
	switch {
	case f.priorCL == nil:
		return false, nil
	case f.priorCL.Snapshot == nil:
		// CL is likely created as a dependency and not yet populated.
		return false, nil

	case f.priorCL.Snapshot.GetGerrit().GetInfo() == nil:
		panic(errors.Reason("%s has snapshot without Gerrit Info", f).Err())
	case f.priorCL.ApplicableConfig == nil:
		panic(errors.Reason("%s has snapshot but not ApplicableConfig", f).Err())

	case f.updatedHint.IsZero():
		return false, nil
	case f.priorCL.Snapshot.IsUpToDate(f.luciProject, f.updatedHint):
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
			return true, changelist.Update(ctx, "", f.priorCL.ID, changelist.UpdateFields{
				ApplicableConfig: acfg,
			})
		}
	}
	return false, nil
}

func (f *fetcher) update(ctx context.Context) (err error) {
	if f.g, err = gerrit.CurrentClient(ctx, f.host, f.luciProject); err != nil {
		return err
	}

	f.newSnapshot = &changelist.Snapshot{Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{}}}
	// TODO(tandrii): optimize for existing CL case.
	if err := f.new(ctx); err != nil {
		return err
	}

	if err := f.resolveDeps(ctx); err != nil {
		return err
	}

	min, cur, err := gerrit.EquivalentPatchsetRange(f.newSnapshot.GetGerrit().GetInfo())
	if err != nil {
		return err
	}
	f.newSnapshot.MinEquivalentPatchset = int32(min)
	f.newSnapshot.Patchset = int32(cur)
	return changelist.Update(ctx, f.externalID, f.clidIfKnown(), changelist.UpdateFields{
		Snapshot:         f.newSnapshot,
		ApplicableConfig: f.newAcfg,
	})
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

		return gerrit.UnhandledError(ctx, err, "failed to fetch %s", f)
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
	// Meanwhile, compute soft deps. Currently, it's cheap operation.
	// In the future, it may require sending another RPC to Gerrit,
	// e.g. to fetch related CLs by topic.
	if err = f.setSoftDeps(); err != nil {
		return err
	}
	if err = eg.Wait(); err != nil {
		return err
	}
	return nil
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
		return gerrit.UnhandledError(ctx, err, "failed to fetch related changes for %s", f)
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
		return gerrit.UnhandledError(ctx, err, "failed to fetch files for %s", f)
	}
}

// setSoftDeps parses CL description and sets soft deps.
func (f *fetcher) setSoftDeps() error {
	ci := f.newSnapshot.GetGerrit().GetInfo()
	msg := ci.GetRevisions()[ci.GetCurrentRevision()].GetCommit().GetMessage()
	deps := cqdepend.Parse(msg)
	if len(deps) == 0 {
		return nil
	}

	// Given f.host like "sub-review.x.y.z", compute "-review.x.y.z" suffix.
	dot := strings.IndexRune(f.host, '.')
	if dot == -1 || !strings.HasSuffix(f.host[:dot], "-review") {
		return errors.Reason("Host %s doesn't support Cq-Depend (%s)", f.host, f).Err()
	}
	hostSuffix := f.host[dot-len("-review"):]

	softDeps := make([]*changelist.GerritSoftDep, len(deps))
	for i, d := range deps {
		depHost := f.host
		if d.Subdomain != "" {
			depHost = d.Subdomain + hostSuffix
		}
		softDeps[i] = &changelist.GerritSoftDep{Host: depHost, Change: int64(d.Change)}
	}
	f.newSnapshot.GetGerrit().SoftDeps = softDeps
	return nil
}

// resolveDeps resolves to CLID and triggers tasks for each of the soft and GerritGit dep.
func (f *fetcher) resolveDeps(ctx context.Context) error {
	eids, err := f.depsToExternalIDs()
	if err != nil {
		return err
	}

	lock := sync.Mutex{}
	resolved := make([]*changelist.Dep, 0, len(eids))

	addDep := func(depCL *changelist.CL, eid changelist.ExternalID, kind changelist.DepKind) error {
		lock.Lock()
		resolved = append(resolved, &changelist.Dep{Clid: int64(depCL.ID), Kind: kind})
		lock.Unlock()

		switch yes, err := depCL.NeedsFetching(ctx, f.luciProject); {
		case err != nil:
			return err
		case yes:
			depHost, depChange, err := eid.ParseGobID()
			if err != nil {
				panic("impossible: by construction, all deps are Gerrit, too")
			}
			return Schedule(ctx, f.luciProject, depHost, depChange, time.Time{}, depCL.ID)
		}
		return nil
	}

	// TODO(tandrii): optimize for the typical case where each dep is already known
	// to CV by sending just 1 multi-Get against CLMap before doing parallel
	// GetOrInsert calls.

	errs := parallel.WorkPool(10, func(work chan<- func() error) {
		for eid, kind := range eids {
			eid, kind := eid, kind
			work <- func() error {
				depCL, err := eid.GetOrInsert(ctx, func(*changelist.CL) {
					// TODO(tandrii): somehow record when CL was inserted,
					// to put a boundary on how long ProjectManager should wait for
					// dependency to be fetched.
				})
				if err != nil {
					return err
				}
				return addDep(depCL, eid, kind)
			}
		}
	})
	if errs != nil {
		// All errors must be transient. Return any one of them.
		return errs.(errors.MultiError).First()
	}
	f.newSnapshot.Deps = resolved
	return nil
}

func (f *fetcher) depsToExternalIDs() (map[changelist.ExternalID]changelist.DepKind, error) {
	cqdeps := f.newSnapshot.GetGerrit().GetSoftDeps()
	gitdeps := f.newSnapshot.GetGerrit().GetGitDeps()
	// Git deps that are immediate parents of the current CL are HARD deps.
	// Since arbitrary Cq-Depend deps may duplicate those of Git,
	// avoid accidental downgrading from HARD to SOFT dep by processing Cq-Depend
	// first and Git deps second.
	eids := make(map[changelist.ExternalID]changelist.DepKind, len(cqdeps)+len(gitdeps))
	for _, dep := range cqdeps {
		eid, err := changelist.GobID(dep.Host, dep.Change)
		if err != nil {
			return nil, err
		}
		eids[eid] = changelist.DepKind_SOFT
	}
	for _, dep := range gitdeps {
		eid, err := changelist.GobID(f.host, dep.Change)
		if err != nil {
			return nil, err
		}
		kind := changelist.DepKind_SOFT
		if dep.Immediate {
			kind = changelist.DepKind_HARD
		}
		eids[eid] = kind
	}
	return eids, nil
}

// ensureNotStale returns error if given Gerrit updated timestamp is older than
// the updateHint or existing CL state.
func (f *fetcher) ensureNotStale(ctx context.Context, externalUpdateTime *timestamppb.Timestamp) error {
	t := externalUpdateTime.AsTime()
	storedTS := f.priorSnapshot().GetExternalUpdateTime()

	switch {
	case !f.updatedHint.IsZero() && f.updatedHint.After(t):
		logging.Errorf(ctx, "Fetched last Gerrit update of %s, but %s expected", t, f.updatedHint)
	case storedTS != nil && storedTS.AsTime().After(t):
		logging.Errorf(ctx, "Fetched last Gerrit update of %s, but %s was already seen & stored", t, storedTS.AsTime())
	default:
		return nil
	}
	return errors.Reason("Fetched stale Gerrit data").Tag(transient.Tag).Err()
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
