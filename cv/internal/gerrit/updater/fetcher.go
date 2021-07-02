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

package updater

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/cqdepend"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap"
)

const (
	// noAccessGraceDuration works around eventually consistent Gerrit,
	// whereby Gerrit can temporarily return 404 for a CL that actually exists.
	noAccessGraceDuration = 10 * time.Minute

	// noAccessGraceRetryDelay determines when to schedule next retry task.
	noAccessGraceRetryDelay = 1 * time.Minute

	// autoRefreshAfter makes CLs worthy of "blind" refresh.
	//
	// "blind" refresh means that CL is already stored in Datastore and is up to
	// the date to the best knowledge of CV.
	autoRefreshAfter = 2 * time.Hour
)

// fetcher efficiently computes new snapshot by fetching data from Gerrit.
//
// It ensures each dependency is resolved to an existing CLID,
// creating CLs in datastore as needed. Schedules tasks to update
// dependencies but doesn't wait for them to complete.
//
// The prior Snapshot, if given, can reduce RPCs made to Gerrit.
type fetcher struct {
	pm              PM
	rm              RM
	scheduleRefresh func(ctx context.Context, p *RefreshGerritCL, delay time.Duration) error

	luciProject string
	host        string
	change      int64
	updatedHint time.Time
	forceNotify bool

	g gerrit.Client

	externalID changelist.ExternalID
	priorCL    *changelist.CL

	toUpdate changelist.UpdateFields
}

func (f *fetcher) update(ctx context.Context, clidHint common.CLID) (err error) {
	// Check if CL already exists in Datastore.
	if clidHint != 0 {
		f.priorCL = &changelist.CL{ID: clidHint}
		err = datastore.Get(ctx, f.priorCL)
	} else {
		f.priorCL, err = f.externalID.Get(ctx)
	}

	switch {
	case err == datastore.ErrNoSuchEntity:
		if clidHint != 0 {
			return errors.Reason("clidHint %d doesn't refer to an existing CL (%s)", clidHint, f).Err()
		}
		f.priorCL = nil
		err = f.fetchNew(ctx)

	case err != nil:
		return err

	case f.priorCL.Snapshot == nil:
		// CL exists, but without snapshot, usually because it was created as
		// a dependency of another CL.
		err = f.fetchNew(ctx)

	case needsRefresh(ctx, f.priorCL, f.luciProject) || f.updatedHint.IsZero():
		logging.Debugf(ctx, "force updating %s", f)
		err = f.fetchExisting(ctx)

	case f.updatedHint.After(f.priorCL.Snapshot.GetExternalUpdateTime().AsTime()):
		// Either force update or updatedHint is after the snapshot we already have.

		// NOTE: ideally, we'd check whether the current project is watching the
		// (host,repo,ref) which is stored in a snapshot. Unfortunately, ref is not
		// immutable after a change creation, see Gerrit move API which is actually
		// being used to transition to the main branch:
		// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#move-change
		// Therefore, here, proceed to fetchng snapshot.
		logging.Debugf(ctx, "updating %s to at least %s", f, f.updatedHint)
		err = f.fetchExisting(ctx)

	default:
		// Snapshot considered up-to-date, check if we can skip updates.
		ci := f.priorCL.Snapshot.GetGerrit().GetInfo()
		var acfg *changelist.ApplicableConfig
		switch acfg, err = gobmap.Lookup(ctx, f.host, ci.GetProject(), ci.GetRef()); {
		case err != nil:
			return err
		case !f.priorCL.ApplicableConfig.SemanticallyEqual(acfg):
			// Only update CL.ApplicableConfig in datastore iff it's materially different,
			// since timestamp is almost guaranteed to be newer.
			f.toUpdate.ApplicableConfig = acfg
		}
		switch {
		case acfg.HasOnlyProject(f.luciProject) && f.priorCL.Snapshot.GetLuciProject() != f.luciProject:
			// Snapshot considered up-to-date, but fetched in the context of a wrong
			// project. Must re-fetch. It's OK to re-use prior snapshot so long as
			// read access to Gerrit is verified.
			logging.Warningf(ctx, "%s switches from %q to %q LUCI project", f, f.priorCL.Snapshot.GetLuciProject(), f.luciProject)
			err = f.fetchExisting(ctx)
		case f.priorCL.Access.GetByProject()[f.luciProject] != nil:
			logging.Debugf(ctx, "%s had access restriction before for %s", f, f.priorCL.Access.GetByProject()[f.luciProject])
			err = f.fetchExisting(ctx)
		default:
			logging.Debugf(ctx, "%s skipping fetching Snapshot from Gerrit", f)
		}
	}

	switch {
	case err != nil:
		return err
	case f.toUpdate.IsEmpty():
		if f.priorCL == nil {
			panic("update can't be skipped iff priorCL is not set")
		}
		if f.forceNotify {
			return f.notify(ctx, f.priorCL)
		}
		return nil
	default:
		return changelist.Update(ctx, f.externalID, f.clidIfKnown(), f.toUpdate, f.notify)
	}
}

func (f *fetcher) notify(ctx context.Context, cl *changelist.CL) error {
	eg, ectx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return f.pm.NotifyCLUpdated(ectx, f.luciProject, cl.ID, cl.EVersion)
	})
	// Generally, a CL will have only one Run at a time. Hence, use
	// unbounded parallelism here.
	for _, rid := range cl.IncompleteRuns {
		rid := rid
		eg.Go(func() error {
			return f.rm.NotifyCLUpdated(ectx, rid, cl.ID, cl.EVersion)
		})
	}
	return eg.Wait()
}

// fetchExisting efficiently fetches new snapshot from Gerrit,
// but it may re-use data from prior snapshot.
func (f *fetcher) fetchExisting(ctx context.Context) error {
	// TODO(tandrii): actually do this efficiently.
	return f.fetchNew(ctx)
}

// fetchNew efficiently fetches fetchNew snapshot from Gerrit.
func (f *fetcher) fetchNew(ctx context.Context) error {
	ci, err := f.fetchChangeInfo(ctx,
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
	)
	if err != nil || ci == nil {
		return err
	}
	changelist.RemoveUnusedGerritInfo(ci)

	f.toUpdate.Snapshot = &changelist.Snapshot{
		LuciProject:        f.luciProject,
		ExternalUpdateTime: ci.GetUpdated(),
		Kind: &changelist.Snapshot_Gerrit{
			Gerrit: &changelist.Gerrit{
				Host: f.host,
				Info: ci,
			},
		},
	}
	f.toUpdate.DelAccess = []string{f.luciProject}
	return f.fetchPostChangeInfo(ctx, ci)
}

func (f *fetcher) fetchPostChangeInfo(ctx context.Context, ci *gerritpb.ChangeInfo) error {
	min, cur, err := gerrit.EquivalentPatchsetRange(ci)
	if err != nil {
		return errors.Annotate(err, "failed to compute equivalent patchset range on %s", f).Err()
	}
	f.toUpdate.Snapshot.MinEquivalentPatchset = int32(min)
	f.toUpdate.Snapshot.Patchset = int32(cur)

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

	if f.priorSnapshot().GetGerrit().GetInfo().GetCurrentRevision() == f.mustHaveCurrentRevision() {
		// Re-use past results since CurrentRevision is the same.
		f.toUpdate.Snapshot.GetGerrit().Files = f.priorSnapshot().GetGerrit().GetFiles()
		f.toUpdate.Snapshot.GetGerrit().GitDeps = f.priorSnapshot().GetGerrit().GetGitDeps()
		// NOTE: CQ-Depend deps are fixed per revision. Once soft deps are accepted
		// via hashtags or topics, the re-use won't be possible.
		f.toUpdate.Snapshot.GetGerrit().SoftDeps = f.priorSnapshot().GetGerrit().GetSoftDeps()
	} else {
		eg, ectx := errgroup.WithContext(ctx)
		eg.Go(func() error { return f.fetchFiles(ectx) })
		eg.Go(func() error { return f.fetchRelated(ectx) })
		// Meanwhile, compute soft deps. Currently, it's cheap operation.
		// In the future, it may require sending another RPC to Gerrit,
		// e.g. to fetch related CLs by topic.
		if err = f.setSoftDeps(); err != nil {
			return err
		}
		if err = eg.Wait(); err != nil {
			return err
		}
	}

	// Always run resolveDeps regardless of re-use of GitDeps/SoftDeps.
	// CV retention policy deletes CLs not modified for a long time,
	// which in some very rare case may affect a dep of this CL.
	// TODO(tandrii): remove such risk by force-updating dep CLs to prevent
	// retention policy from wiping them out.
	if err := f.resolveDeps(ctx); err != nil {
		return err
	}
	return nil
}

// fetchChangeInfo fetches newest ChangeInfo from Gerrit.
//
// * handles permission errors
// * verifies fetched data isn't definitely stale.
// * checks that current LUCI project is still watching the change.
//
// Returns nil ChangeInfo if no further fetching should proceed.
func (f *fetcher) fetchChangeInfo(ctx context.Context, opts ...gerritpb.QueryOption) (*gerritpb.ChangeInfo, error) {
	setNoAccess := func(temporary bool) error {
		now := clock.Now(ctx)
		noAccessAt := now
		if temporary {
			noAccessAt = now.Add(noAccessGraceDuration)
			t := &RefreshGerritCL{
				LuciProject: f.luciProject,
				Host:        f.host,
				Change:      f.change,
				ClidHint:    int64(f.clidIfKnown()),
			}
			if !f.updatedHint.IsZero() {
				t.UpdatedHint = timestamppb.New(f.updatedHint)
			}
			err := f.scheduleRefresh(ctx, t, noAccessGraceRetryDelay)
			if err != nil {
				return err
			}
		}
		f.toUpdate.AddDependentMeta = &changelist.Access{
			ByProject: map[string]*changelist.Access_Project{
				f.luciProject: {
					UpdateTime:   timestamppb.New(now),
					NoAccessTime: timestamppb.New(noAccessAt),
					NoAccess:     true,
				},
			},
		}
		return nil
	}

	// Avoid querying Gerrit iff the current project doesn't watch the given host,
	// which should be treated as PermissionDenied.
	switch watched, err := f.isHostWatched(ctx); {
	case err != nil:
		return nil, err
	case !watched:
		logging.Warningf(ctx, "Gerrit host %q is not watched by project %q [%s]", f.host, f.luciProject, f)
		return nil, setNoAccess(false /* permanent*/)
	}

	if err := f.ensureGerritClient(ctx); err != nil {
		return nil, err
	}
	ci, err := f.g.GetChange(ctx, &gerritpb.GetChangeRequest{
		Number:  f.change,
		Project: f.gerritProjectIfKnown(),
		Options: opts,
	})
	switch grpcutil.Code(err) {
	case codes.OK:
		if err := f.ensureNotStale(ctx, ci.GetUpdated()); err != nil {
			return nil, err
		}
	case codes.NotFound, codes.PermissionDenied:
		// Either no access OR CL was deleted OR eventual consistency.
		return nil, setNoAccess(true /* temporary */)
	case codes.ResourceExhausted:
		return nil, errOutOfQuota
	default:
		return nil, gerrit.UnhandledError(ctx, err, "failed to fetch %s", f)
	}

	f.toUpdate.ApplicableConfig, err = gobmap.Lookup(ctx, f.host, ci.GetProject(), ci.GetRef())
	switch {
	case err != nil:
		return nil, err
	case !f.toUpdate.ApplicableConfig.HasProject(f.luciProject):
		logging.Debugf(ctx, "%s is not watched by the %q project", f, f.luciProject)
		return nil, nil
	}

	return ci, nil
}

// fetchRelated fetches related changes and computes GerritGitDeps.
func (f *fetcher) fetchRelated(ctx context.Context) error {
	if err := f.ensureGerritClient(ctx); err != nil {
		return err
	}
	resp, err := f.g.GetRelatedChanges(ctx, &gerritpb.GetRelatedChangesRequest{
		Number:     f.change,
		Project:    f.gerritProjectIfKnown(),
		RevisionId: f.mustHaveCurrentRevision(),
	})
	switch code := grpcutil.Code(err); code {
	case codes.OK:
		f.setGitDeps(ctx, resp.GetChanges())
		return nil
	case codes.PermissionDenied, codes.NotFound:
		// Getting this right after successfully fetching ChangeInfo should
		// typically be due to eventual consistency of Gerrit, and rarely due to
		// change of ACLs. So, err transiently s.t. retry handles the same error
		// when re-fetching ChangeInfo.
		return errors.Annotate(err, "failed to fetch related changes for %s", f).Tag(transient.Tag).Err()
	case codes.ResourceExhausted:
		return errOutOfQuota
	default:
		return gerrit.UnhandledError(ctx, err, "failed to fetch related changes for %s", f)
	}
}

// setGitDeps sets GerritGitDeps based on list of related changes provided by
// Gerrit.GetRelatedChanges RPC.
//
// If GetRelatedChanges output is invalid, doesn't set GerritGitDep and adds an
// appropriate CLError to Snapshot.Errors.
func (f *fetcher) setGitDeps(ctx context.Context, related []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit) {
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
		return
	}
	this, clErr := f.matchCurrentAmongRelated(ctx, related)
	if clErr != nil {
		f.toUpdate.Snapshot.Errors = append(f.toUpdate.Snapshot.Errors, clErr)
		return
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
		return
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
					logging.Warningf(
						ctx,
						"Gerrit.GetRelatedChanges returned rev %q %d times for %s (ALL Related %s)",
						p.GetId(), len(prs), f, related)
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
	f.toUpdate.Snapshot.GetGerrit().GitDeps = deps
}

func (f *fetcher) matchCurrentAmongRelated(
	ctx context.Context, related []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit,
) (*gerritpb.GetRelatedChangesResponse_ChangeAndCommit, *changelist.CLError) {

	var this *gerritpb.GetRelatedChangesResponse_ChangeAndCommit
	matched := 0
	for _, r := range related {
		if r.GetNumber() == f.change {
			matched++
			this = r
		}
	}
	if matched != 1 {
		// Apparently in rare cases, Gerrit may get confused and substitute this CL
		// for some other CL in the output (see https://crbug.com/1199471).
		msg := fmt.Sprintf(
			("Gerrit related changes should return the %s/%d CL itself exactly once, but got %d." +
				" Maybe https://crbug.com/1199471 is affecting you?"), f.host, f.change, matched)
		logging.Errorf(ctx, "%s Related output: %s", msg, related)
		return nil, &changelist.CLError{
			Kind: &changelist.CLError_CorruptGerritMetadata{
				CorruptGerritMetadata: msg,
			},
		}
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
	if err := f.ensureGerritClient(ctx); err != nil {
		return err
	}
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
		f.toUpdate.Snapshot.GetGerrit().Files = fs
		return nil

	case codes.PermissionDenied, codes.NotFound:
		// Getting this right after successfully fetching ChangeInfo should
		// typically be due to eventual consistency of Gerrit, and rarely due to
		// change of ACLs. So, err transiently s.t. retry handles the same error
		// when re-fetching ChangeInfo.
		return errors.Annotate(err, "failed to fetch files for %s", f).Tag(transient.Tag).Err()

	case codes.ResourceExhausted:
		return errOutOfQuota

	default:
		return gerrit.UnhandledError(ctx, err, "failed to fetch files for %s", f)
	}
}

// setSoftDeps parses CL description and sets soft deps.
func (f *fetcher) setSoftDeps() error {
	ci := f.toUpdate.Snapshot.GetGerrit().GetInfo()
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
	f.toUpdate.Snapshot.GetGerrit().SoftDeps = softDeps
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

		if needsRefresh(ctx, depCL, f.luciProject) {
			depHost, depChange, err := eid.ParseGobID()
			if err != nil {
				panic("impossible: by construction, all deps are Gerrit, too")
			}
			return f.scheduleRefresh(ctx, &RefreshGerritCL{
				LuciProject: f.luciProject,
				Host:        depHost,
				Change:      depChange,
				ClidHint:    int64(depCL.ID),
			}, 0 /*no delay*/)
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

	sort.Slice(resolved, func(i, j int) bool {
		return resolved[i].GetClid() < resolved[j].GetClid()
	})
	f.toUpdate.Snapshot.Deps = resolved
	return nil
}

func (f *fetcher) depsToExternalIDs() (map[changelist.ExternalID]changelist.DepKind, error) {
	cqdeps := f.toUpdate.Snapshot.GetGerrit().GetSoftDeps()
	gitdeps := f.toUpdate.Snapshot.GetGerrit().GetGitDeps()
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
		logging.Warningf(ctx, "Fetched last Gerrit update of %s, but %s expected (%s)", t, f.updatedHint, f)
	case storedTS != nil && storedTS.AsTime().After(t):
		logging.Warningf(ctx, "Fetched last Gerrit update of %s, but %s was already seen & stored (%s)", t, storedTS.AsTime(), f)
	default:
		return nil
	}
	return errStaleData
}

func (f *fetcher) ensureGerritClient(ctx context.Context) error {
	if f.g != nil {
		return nil
	}
	var err error
	f.g, err = gerrit.CurrentClient(ctx, f.host, f.luciProject)
	return err
}

// Checks whether this LUCI project watches any repo on this Gerrit host.
func (f *fetcher) isHostWatched(ctx context.Context) (bool, error) {
	meta, err := prjcfg.GetLatestMeta(ctx, f.luciProject)
	if err != nil {
		return false, err
	}
	cgs, err := meta.GetConfigGroups(ctx)
	if err != nil {
		return false, err
	}
	for _, cg := range cgs {
		for _, g := range cg.Content.GetGerrit() {
			if prjcfg.GerritHost(g) == f.host {
				return true, nil
			}
		}
	}
	return false, nil
}

func (f *fetcher) gerritProjectIfKnown() string {
	if project := f.priorSnapshot().GetGerrit().GetInfo().GetProject(); project != "" {
		return project
	}
	if project := f.toUpdate.Snapshot.GetGerrit().GetInfo().GetProject(); project != "" {
		return project
	}
	return ""
}

func (f *fetcher) clidIfKnown() common.CLID {
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

func (f *fetcher) mustHaveCurrentRevision() string {
	switch ci := f.toUpdate.Snapshot.GetGerrit().GetInfo(); {
	case ci == nil:
		panic("ChangeInfo must be already fetched into toUpdate.Snapshot")
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

// needsRefresh returns true if CL
func needsRefresh(ctx context.Context, cl *changelist.CL, luciProject string) bool {
	switch {
	case cl == nil:
		panic("dep must be not nil")
	case cl.Snapshot == nil:
		return true
	case cl.Snapshot.GetOutdated() != nil:
		return true
	case cl.Snapshot.GetLuciProject() != luciProject:
		return true
	case clock.Since(ctx, cl.UpdateTime) > autoRefreshAfter:
		// Strictly speaking, cl.UpdateTime isn't just changed on refresh, but also
		// whenever Run starts/ends. However, the start of Run is usually
		// happenening right after recent refresh, and end of Run is usually
		// followed by the refresh.
		return true
	default:
		return false
	}
}
