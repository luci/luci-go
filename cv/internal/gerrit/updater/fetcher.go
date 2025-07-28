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
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/cqdepend"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap"
	"go.chromium.org/luci/cv/internal/gerrit/metadata"
)

const (
	// noAccessGraceDuration works around eventually consistent Gerrit,
	// whereby Gerrit can temporarily return 404 for a CL that actually exists.
	noAccessGraceDuration = 1 * time.Minute

	// noAccessGraceRetryDelay determines when to schedule the next retry task.
	//
	// Set it at approximately ~2 tries before noAccessGraceDuration expires.
	noAccessGraceRetryDelay = noAccessGraceDuration / 3
)

var errStaleOrNoAccess = errors.Fmt("either no access or deleted or stale: %w", gerrit.ErrStaleData)

// fetcher efficiently computes new snapshot by fetching data from Gerrit.
//
// It ensures each dependency is resolved to an existing CLID,
// creating CLs in the Datastore as needed. Schedules tasks to update
// dependencies but doesn't wait for them to complete.
//
// fetch is a single-use object:
//
//	f := fetcher{...}
//	if err := f.fetch(ctx); err != nil {...}
//	// Do something with `f.toUpdate`.
type fetcher struct {
	// Dependencies & input. Must be set.
	gFactory                     gerrit.Factory
	g                            gerrit.Client
	scheduleRefresh              func(context.Context, *changelist.UpdateCLTask, time.Duration) error
	resolveAndScheduleDepsUpdate func(ctx context.Context, project string, deps map[changelist.ExternalID]changelist.DepKind, requester changelist.UpdateCLTask_Requester) ([]*changelist.Dep, error)
	project                      string
	host                         string
	change                       int64
	hint                         *changelist.UpdateCLTask_Hint
	requester                    changelist.UpdateCLTask_Requester
	externalID                   changelist.ExternalID
	priorCL                      *changelist.CL // not-nil, if CL already exists in Datastore.

	// Result is stored here.
	toUpdate changelist.UpdateFields
}

func (f *fetcher) fetch(ctx context.Context) error {
	ci, err := f.fetchChangeInfo(ctx,
		// These are expensive to compute for Gerrit,
		// CV should not do this needlessly.
		gerritpb.QueryOption_ALL_COMMITS,
		gerritpb.QueryOption_ALL_REVISIONS,
		gerritpb.QueryOption_DETAILED_LABELS,
		gerritpb.QueryOption_DETAILED_ACCOUNTS,
		gerritpb.QueryOption_MESSAGES,
		gerritpb.QueryOption_SUBMITTABLE,
		gerritpb.QueryOption_SUBMIT_REQUIREMENTS,
		// Avoid asking Gerrit to perform expensive operation.
		gerritpb.QueryOption_SKIP_MERGEABLE,
	)
	switch {
	case err != nil:
		return err
	case ci == nil:
		// Don't proceed to fetching the other details.
		// It's likely due to one of the following.
		// - CV lacks access to the CL
		// - this LUCI project is no longer watching the CL
		// - the task was hinted with an old MetaRevID due to a pubsub message
		// delivered out of order
		return nil
	}

	f.toUpdate.Snapshot = &changelist.Snapshot{
		LuciProject:        f.project,
		ExternalUpdateTime: ci.GetUpdated(),
		Metadata: metadata.Extract(
			ci.GetRevisions()[ci.GetCurrentRevision()].GetCommit().GetMessage(),
		),
		Kind: &changelist.Snapshot_Gerrit{
			Gerrit: &changelist.Gerrit{
				Host: f.host,
				Info: ci,
			},
		},
	}
	f.toUpdate.DelAccess = []string{f.project}
	if err := f.fetchPostChangeInfo(ctx, ci); err != nil {
		return err
	}
	// Finally, remove all info no longer necessary for CV.
	changelist.RemoveUnusedGerritInfo(ci)
	return nil
}

func (f *fetcher) fetchPostChangeInfo(ctx context.Context, ci *gerritpb.ChangeInfo) error {
	min, cur, err := gerrit.EquivalentPatchsetRange(ci)
	if err != nil {
		return errors.Fmt("failed to compute equivalent patchset range on %s: %w", f, err)
	}
	f.toUpdate.Snapshot.MinEquivalentPatchset = int32(min)
	f.toUpdate.Snapshot.Patchset = int32(cur)

	switch ci.GetStatus() {
	case gerritpb.ChangeStatus_NEW:
		// OK, proceed.
	case gerritpb.ChangeStatus_ABANDONED, gerritpb.ChangeStatus_MERGED:
		// CV doesn't care about such CLs beyond their status, so don't fetch
		// additional details to avoid stumbiling into edge cases with how Gerrit
		// treats abandoned and submitted CLs.
		logging.Debugf(ctx, "%s is %s", f, ci.GetStatus())
		return nil
	default:
		logging.Warningf(ctx, "%s has unknown status %d %s", f, ci.GetStatus().Number(), ci.GetStatus().String())
		return nil
	}

	// Check if we can re-use info from previous snapshot.
	reused := false
	switch before := f.priorSnapshot().GetGerrit(); {
	case before == nil:
	case before.GetInfo().GetCurrentRevision() != f.mustHaveCurrentRevision():
	case before.GetInfo().GetStatus() != gerritpb.ChangeStatus_NEW:
	default:
		reused = true
		// Re-use past results since CurrentRevision is the same.
		f.toUpdate.Snapshot.GetGerrit().Files = before.GetFiles()
		// NOTE: CQ-Depend deps are fixed per revision. Once soft deps are accepted
		// via hashtags or topics, the re-use won't be possible.
		f.toUpdate.Snapshot.GetGerrit().SoftDeps = before.GetSoftDeps()
	}

	eg, ectx := errgroup.WithContext(ctx)
	// Always fetch related changes here. It turns out in b/272828859 that
	// Gerrit GetRelatedChange may return inconsistent response if it is called
	// immediately after a CL is updated. Therefore, the first CL Update attempt
	// may incorrectly set the dependencies. By always fetching related changes,
	// we are hoping any subsequent CL Update attempt would receive up-to-date
	// related change infos so that the Dep info is correctly populated.
	eg.Go(func() error { return f.fetchRelated(ectx) })

	if !reused {
		eg.Go(func() error { return f.fetchFiles(ectx) })
		// Meanwhile, compute soft deps. Currently, it's cheap operation.
		// In the future, it may require sending another RPC to Gerrit,
		// e.g. to fetch related CLs by topic.
		if err = f.setSoftDeps(); err != nil {
			return err
		}
	}

	if err = eg.Wait(); err != nil {
		return err
	}
	// Always run resolveDeps regardless of re-use of GitDeps/SoftDeps.
	// CV retention policy deletes CLs not modified for a long time,
	// which in some very rare case may affect a dep of this CL.
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
	// Avoid querying Gerrit iff the current project doesn't watch the given host,
	// which should be treated as PermissionDenied.
	switch watched, err := f.isHostWatched(ctx); {
	case err != nil:
		return nil, err
	case !watched:
		logging.Warningf(ctx, "Gerrit host %q is not watched by project %q [%s]", f.host, f.project, f)
		return nil, f.setCertainNoAccess(ctx)
	}

	var ci *gerritpb.ChangeInfo
	err := f.gFactory.MakeMirrorIterator(ctx).RetryIfStale(func(opt grpc.CallOption) error {
		var err error
		ci, err = f.g.GetChange(ctx, &gerritpb.GetChangeRequest{
			Number:  f.change,
			Project: f.gerritProjectIfKnown(),
			Options: opts,
			Meta:    f.hint.GetMetaRevId(),
		}, opt)
		switch grpcutil.Code(err) {
		case codes.OK:
			if f.isStale(ctx, ci.GetUpdated()) {
				return gerrit.ErrStaleData
			}
			return nil
		case codes.NotFound, codes.PermissionDenied:
			return errStaleOrNoAccess
		// GetChange() returns codes.FailedPrecondition if meta is given
		// but the SHA-1 is not reachable from the serving Gerrit replica.
		//
		// Return errStaleData to retry on it.
		case codes.FailedPrecondition:
			return gerrit.ErrStaleData
		default:
			return gerrit.UnhandledError(ctx, err, "failed to fetch %s", f)
		}
	})
	switch {
	case err == errStaleOrNoAccess:
		// Chances are it's not due to eventual consistency, but be conservative.
		return nil, f.setLikelyNoAccess(ctx)
	case err != nil:
		return nil, err
	}

	f.toUpdate.ApplicableConfig, err = gobmap.Lookup(ctx, f.host, ci.GetProject(), ci.GetRef())
	switch storedTS := f.priorSnapshot().GetExternalUpdateTime(); {
	case err != nil:
		return nil, err
	case !f.toUpdate.ApplicableConfig.HasProject(f.project):
		logging.Debugf(ctx, "%s is not watched by the %q project", f, f.project)
		return nil, f.setCertainNoAccess(ctx)
	case storedTS != nil && storedTS.AsTime().After(ci.GetUpdated().AsTime()):
		// The fetched snapshot is fresh, but older than the prior snapshot.
		// Then, skip updating the snapshot.
		//
		// It can happen pubsub messages were delivered out of order.
		return nil, nil
	}

	return ci, nil
}

func (f *fetcher) setCertainNoAccess(ctx context.Context) error {
	now := clock.Now(ctx)
	noAccessAt := now
	if prior := f.priorNoAccessTime(); !prior.IsZero() && prior.Before(now) {
		// Keep noAccessAt as is.
		noAccessAt = prior
	}
	f.setNoAccessAt(now, noAccessAt)
	return nil
}

func (f *fetcher) setLikelyNoAccess(ctx context.Context) error {
	now := clock.Now(ctx)
	var noAccessAt time.Time
	var err error
	switch prior := f.priorNoAccessTime(); {
	case prior.IsZero():
		// This is the first time CL.
		noAccessAt = now.Add(noAccessGraceDuration)
		err = f.reschedule(ctx, noAccessGraceRetryDelay)
	case prior.Before(now):
		// Keep noAccessAt as is, it's now considered certain.
		noAccessAt = prior
	default:
		// Keep noAccessAt as is, but schedule yet another refresh.
		noAccessAt = prior
		err = f.reschedule(ctx, noAccessGraceRetryDelay)
	}
	f.setNoAccessAt(now, noAccessAt)
	return err
}

func (f *fetcher) setNoAccessAt(now, noAccessAt time.Time) {
	f.toUpdate.AddDependentMeta = &changelist.Access{
		ByProject: map[string]*changelist.Access_Project{
			f.project: {
				UpdateTime:   timestamppb.New(now),
				NoAccessTime: timestamppb.New(noAccessAt),
				NoAccess:     true,
			},
		},
	}
}

// reschedule reschedules the same task with a delay.
func (f *fetcher) reschedule(ctx context.Context, delay time.Duration) error {
	t := &changelist.UpdateCLTask{
		LuciProject: f.project,
		ExternalId:  string(f.externalID),
		Id:          int64(f.clidIfKnown()),
		Hint:        f.hint,
		Requester:   f.requester,
	}
	return f.scheduleRefresh(ctx, t, delay)
}

// fetchRelated fetches related changes and computes GerritGitDeps.
func (f *fetcher) fetchRelated(ctx context.Context) error {
	return f.gFactory.MakeMirrorIterator(ctx).RetryIfStale(func(opt grpc.CallOption) error {
		resp, err := f.g.GetRelatedChanges(ctx, &gerritpb.GetRelatedChangesRequest{
			Number:     f.change,
			Project:    f.gerritProjectIfKnown(),
			RevisionId: f.mustHaveCurrentRevision(),
		}, opt)
		switch code := grpcutil.Code(err); code {
		case codes.OK:
			f.setGitDeps(ctx, resp.GetChanges())
			return nil
		case codes.PermissionDenied, codes.NotFound:
			// Getting this right after successfully fetching ChangeInfo should
			// typically be due to eventual consistency of Gerrit, and rarely due to
			// change of ACLs.
			return gerrit.ErrStaleData
		default:
			return gerrit.UnhandledError(ctx, err, "failed to fetch related changes for %s", f)
		}
	})
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
	return f.gFactory.MakeMirrorIterator(ctx).RetryIfStale(func(opt grpc.CallOption) error {
		curRev := f.mustHaveCurrentRevision()
		req := &gerritpb.ListFilesRequest{
			Number:     f.change,
			Project:    f.gerritProjectIfKnown(),
			RevisionId: curRev,
		}
		switch revInfo, ok := f.toUpdate.Snapshot.GetGerrit().GetInfo().GetRevisions()[curRev]; {
		case !ok:
			return errors.Fmt("missing RevisionInfo for current revision: %s", curRev)
		case len(revInfo.GetCommit().GetParents()) == 0:
			// Occasionally, CL doesn't have parent commit. See: crbug.com/1295817.
		default:
			// For CLs with >1 parent commit (aka merge commits), this relies on
			// Gerrit ensuring that such a CL always has first parent from the
			// target branch.
			req.Parent = 1 // Request a diff against the first parent.
		}
		resp, err := f.g.ListFiles(ctx, req, opt)
		switch code := grpcutil.Code(err); code {
		case codes.OK:
			// Iterate files map and take keys only. CV treats all files "touched" in
			// a Change to be interesting, including chmods. Skip special /COMMIT_MSG
			// and /MERGE_LIST entries, which aren't files. For example output, see
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
			return gerrit.ErrStaleData
		default:
			return gerrit.UnhandledError(ctx, err, "failed to fetch files for %s", f)
		}
	})
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
		return errors.Fmt("Host %s doesn't support Cq-Depend (%s)", f.host, f)
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
	depsMap, err := f.depsToExternalIDs()
	if err != nil {
		return err
	}

	if depsCnt := len(depsMap); depsCnt > 500 {
		logging.Warningf(ctx, "CL has high number of dependency CLs. Deps count: %d", depsCnt)
	}
	resolved, err := f.resolveAndScheduleDepsUpdate(ctx, f.project, depsMap, f.requester)
	if err != nil {
		return err
	}
	f.toUpdate.Snapshot.Deps = resolved
	return nil
}

func (f *fetcher) depsToExternalIDs() (map[changelist.ExternalID]changelist.DepKind, error) {
	cqdeps := f.toUpdate.Snapshot.GetGerrit().GetSoftDeps()
	gitdeps := f.toUpdate.Snapshot.GetGerrit().GetGitDeps()
	// Git deps are HARD deps. Since arbitrary Cq-Depend deps may duplicate those
	// of Git, avoid accidental downgrading from HARD to SOFT dep by processing
	// Cq-Depend first and Git deps second.
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
		eids[eid] = changelist.DepKind_HARD
	}
	return eids, nil
}

// isStale returns true if given Gerrit updated timestamp is older than
// the updateHint or the existing CL state.
func (f *fetcher) isStale(ctx context.Context, externalUpdateTime *timestamppb.Timestamp) bool {
	if f.hint.GetMetaRevId() != "" {
		// If meta was set, it's always the fresh data of the snapshot.
		//
		// If it is an older snapshot than the stored snapshot,
		// the snapshot update will be skipped.
		return false
	}
	t := externalUpdateTime.AsTime()
	storedTS := f.priorSnapshot().GetExternalUpdateTime()
	hintedTS := f.hint.GetExternalUpdateTime()
	switch {
	case hintedTS != nil && hintedTS.AsTime().After(t):
		logging.Debugf(ctx, "Fetched last Gerrit update of %s, but %s expected (%s)", t, hintedTS.AsTime(), f)
	case storedTS != nil && storedTS.AsTime().After(t):
		logging.Debugf(ctx, "Fetched last Gerrit update of %s, but %s was already seen & stored (%s)", t, storedTS.AsTime(), f)
	default:
		return false
	}
	return true
}

// Checks whether this LUCI project watches any repo on this Gerrit host.
func (f *fetcher) isHostWatched(ctx context.Context) (bool, error) {
	meta, err := prjcfg.GetLatestMeta(ctx, f.project)
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

func (f *fetcher) priorNoAccessTime() time.Time {
	if f.priorCL == nil {
		return time.Time{}
	}
	t := f.priorCL.Access.GetByProject()[f.project].GetNoAccessTime()
	if t == nil {
		return time.Time{}
	}
	return t.AsTime()
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
