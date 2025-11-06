// Copyright 2016 The LUCI Authors.
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

package gitiles

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/api/pubsub/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/auth"

	api "go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/gitiles/pb"
)

// gitilesRPCTimeout limits how long Gitiles RPCs are allowed to last.
const gitilesRPCTimeout = time.Minute

// defaultMaxTriggersPerInvocation limits number of triggers emitted per one
// invocation.
const defaultMaxTriggersPerInvocation = 100

// defaultMaxCommitsPerRefUpdate limits number of commits (and hence triggers)
// emitted when a ref changes.
// Must be smaller than defaultMaxTriggersPerInvocation, else these many
// triggers could be emitted.
const defaultMaxCommitsPerRefUpdate = 50

// refsTagsPrefix is the ref namespace prefix reserved for tags. Any ref with
// this prefix is assumed to be a tag.
const refsTagsPrefix = "refs/tags/"

// TaskManager implements task.Manager interface for tasks defined with
// GitilesTask proto message.
type TaskManager struct {
	mockGitilesClient        gitilespb.GitilesClient // Used for testing only.
	maxTriggersPerInvocation int                     // Avoid choking on DS or runtime limits.
	maxCommitsPerRefUpdate   int                     // Failsafe when someone pushes too many commits at once.
}

// Name is part of Manager interface.
func (m TaskManager) Name() string {
	return "gitiles"
}

// ProtoMessageType is part of Manager interface.
func (m TaskManager) ProtoMessageType() proto.Message {
	return (*messages.GitilesTask)(nil)
}

// Traits is part of Manager interface.
func (m TaskManager) Traits() task.Traits {
	return task.Traits{
		Multistage: false, // we don't use task.StatusRunning state
	}
}

// ValidateProtoMessage is part of Manager interface.
func (m TaskManager) ValidateProtoMessage(c *validation.Context, msg proto.Message, realmID string) {
	cfg, ok := msg.(*messages.GitilesTask)
	if !ok {
		c.Errorf("wrong type %T, expecting *messages.GitilesTask", msg)
		return
	}

	// Validate 'repo' field.
	c.Enter("repo")
	if cfg.Repo == "" {
		c.Errorf("field 'repository' is required")
	} else {
		u, err := url.Parse(cfg.Repo)
		if err != nil {
			c.Errorf("invalid URL %q: %s", cfg.Repo, err)
		} else if !u.IsAbs() {
			c.Errorf("not an absolute url: %q", cfg.Repo)
		}
	}
	c.Exit()

	c.Enter("refs")
	gitiles.ValidateRefSet(c, cfg.Refs)
	c.Exit()

	validatePathRegexp := func(p string) {
		if p == "" {
			c.Errorf("must not be empty")
		}
		if strings.HasPrefix(p, "^") || strings.HasSuffix(p, "$") {
			c.Errorf("^ and $ qualifiers are added automatically, please remove them")
		}
		_, err := regexp.Compile(p)
		if err != nil {
			c.Errorf("%s", err)
		}
	}

	for _, p := range cfg.PathRegexps {
		c.Enter("path_regexps %q", p)
		validatePathRegexp(p)
		c.Exit()
	}
	if len(cfg.PathRegexpsExclude) > 0 && len(cfg.PathRegexps) == 0 {
		c.Errorf("Specifying path_regexps_exclude not allowed without at least one path_regexps")
	} else {
		for _, p := range cfg.PathRegexpsExclude {
			c.Enter("path_regexps_exclude %q", p)
			validatePathRegexp(p)
			c.Exit()
		}
	}
}

// LaunchTask is part of Manager interface.
func (m TaskManager) LaunchTask(c context.Context, ctl task.Controller) error {
	cfg := ctl.Task().(*messages.GitilesTask)
	ctl.DebugLog("Repo: %s, Refs: %s", cfg.Repo, cfg.Refs)

	g, err := m.getGitilesClient(c, ctl, cfg.Repo)
	if err != nil {
		return err
	}
	refs, err := m.fetchRefsState(c, ctl, cfg, g)
	if err != nil {
		ctl.DebugLog("Error fetching state of the world: %s", err)
		return err
	}

	refs.pruneKnown(ctl)
	leftToProcess, err := m.emitTriggersRefAtATime(c, ctl, g, cfg, refs)

	if err != nil {
		switch {
		case leftToProcess == 0:
			panic(err) // leftToProcess must include the one processing of which failed.
		case refs.changed > 0:
			// Even though we hit error, we had progress. So, ignore error as transient.
			ctl.DebugLog("ignoring error %s as transient", err)
		default:
			ctl.DebugLog("no progress made due to %s", err)
			return err
		}
	}

	switch {
	case leftToProcess > 0 && refs.changed == 0:
		panic(errors.New("no progress with no errors must not happen"))
	case refs.changed == 0:
		ctl.DebugLog("No changes detected")
		ctl.State().Status = task.StatusSucceeded
		return nil
	case leftToProcess > 0:
		ctl.DebugLog("%d changed refs processed, %d refs not yet examined", refs.changed, leftToProcess)
	default:
		ctl.DebugLog("All %d changed refs processed", refs.changed)
	}
	// Force save to ensure triggers are actually emitted.
	if err := ctl.Save(c); err != nil {
		// At this point, triggers have not been sent, so bail now and don't save
		// the refs' heads newest values.
		return err
	}
	if err := saveState(c, ctl.JobID(), cfg, refs.known); err != nil {
		return err
	}
	ctl.DebugLog("Saved %d known refs", len(refs.known))
	ctl.State().Status = task.StatusSucceeded
	return nil
}

// AbortTask is part of Manager interface.
func (m TaskManager) AbortTask(c context.Context, ctl task.Controller) error {
	return nil
}

// ExamineNotification is part of Manager interface.
func (m TaskManager) ExamineNotification(c context.Context, msg *pubsub.PubsubMessage) string {
	return ""
}

// HandleNotification is part of Manager interface.
func (m TaskManager) HandleNotification(c context.Context, ctl task.Controller, msg *pubsub.PubsubMessage) error {
	return errors.New("not implemented")
}

// HandleTimer is part of Manager interface.
func (m TaskManager) HandleTimer(c context.Context, ctl task.Controller, name string, payload []byte) error {
	return errors.New("not implemented")
}

// GetDebugState is part of Manager interface.
func (m TaskManager) GetDebugState(c context.Context, ctl task.ControllerReadOnly) (*internal.DebugManagerState, error) {
	cfg := ctl.Task().(*messages.GitilesTask)
	g, err := m.getGitilesClient(c, ctl, cfg.Repo)
	if err != nil {
		return nil, err
	}

	refs, err := m.fetchRefsState(c, ctl, cfg, g)
	if err != nil {
		ctl.DebugLog("Error fetching state of the world: %s", err)
		return nil, err
	}

	sortedRefs := func(refs map[string]string) []*pb.DebugState_Ref {
		keys := make([]string, 0, len(refs))
		for k := range refs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]*pb.DebugState_Ref, len(refs))
		for i, key := range keys {
			out[i] = &pb.DebugState_Ref{Ref: key, Commit: refs[key]}
		}
		return out
	}

	return &internal.DebugManagerState{
		GitilesPoller: &pb.DebugState{
			Known:   sortedRefs(refs.known),
			Current: sortedRefs(refs.current),
			Search:  cfg.GetRefs(),
		},
	}, nil
}

func (m TaskManager) fetchRefsState(c context.Context, ctl task.ControllerReadOnly, cfg *messages.GitilesTask, g *gitilesClient) (*refsState, error) {
	refs := &refsState{}
	refs.watched = gitiles.NewRefSet(cfg.GetRefs())
	var missingRefs []string
	err := parallel.FanOutIn(func(work chan<- func() error) {
		work <- func() (loadErr error) {
			var previousRefs []string
			refs.known, previousRefs, loadErr = loadState(c, ctl.JobID(), cfg.Repo)
			refs.previousWatched = gitiles.NewRefSet(previousRefs)
			return
		}
		work <- func() (resolveErr error) {
			c, cancel := clock.WithTimeout(c, gitilesRPCTimeout)
			defer cancel()
			refs.current, missingRefs, resolveErr = refs.watched.Resolve(c, g, g.project)
			return
		}
	})
	if err != nil {
		return nil, err
	}
	ctl.DebugLog("Fetched refs: %d from datastore, %d from gitiles", len(refs.known), len(refs.current))
	if len(missingRefs) > 0 {
		sort.Strings(missingRefs)
		ctl.DebugLog("The following configured refs didn't match a single actual ref: %s\n"+
			"Hint: have you granted read access to LUCI Scheduler on relevant refs in %q\n"+
			"Resolved refs from gitiles:\n%q?\n",
			missingRefs, cfg.Repo, refs.current)
		return nil, errors.Fmt("%d unresolved refs", len(missingRefs))
	}
	return refs, nil
}

// emitTriggersRefAtATime processes refs one a time and emits triggers if ref
// changed. Limits number of triggers emitted and so may stop early, but
// processes each ref atomically: either all triggers for it are emittted or
// none.
//
// Returns how many refs were not examined.
func (m TaskManager) emitTriggersRefAtATime(c context.Context, ctl task.Controller, g *gitilesClient,
	cfg *messages.GitilesTask, refs *refsState) (int, error) {
	// Safeguard against too many changes such as the first run after config
	// change to watch many more refs than before.
	maxTriggersPerInvocation := m.maxTriggersPerInvocation
	if maxTriggersPerInvocation <= 0 {
		maxTriggersPerInvocation = defaultMaxTriggersPerInvocation
	}
	maxCommitsPerRefUpdate := m.maxCommitsPerRefUpdate
	if maxCommitsPerRefUpdate <= 0 {
		maxCommitsPerRefUpdate = defaultMaxCommitsPerRefUpdate
	}
	emittedTriggers := 0
	// Note, that refs.current contain only watched refs (see getRefsTips).
	// For determinism, sort refs by name.
	sortedRefs := refs.sortedCurrentRefNames()
	pathFilter, err := newPathFilter(cfg)
	if err != nil {
		return 0, err
	}
	for i, ref := range sortedRefs {
		commits, _, err := refs.newCommits(c, ctl, g, ref, maxCommitsPerRefUpdate, pathFilter)
		if err != nil {
			// This ref counts as not yet examined.
			return len(sortedRefs) - i, err
		}

		// For tags, only consider the tag be new if it was also matched by the
		// last-used refset. If not, the tag is probably actually not new, and
		// instead the task itself is new, or its configured refs have changed
		// so that they match a different set of tags than before.
		if strings.HasPrefix(ref, refsTagsPrefix) && !refs.previousWatched.Has(ref) {
			ctl.DebugLog("Tag %s newly matched but already existed", ref)
			continue
		}

		for i := range commits {
			// commit[0] is latest, so emit triggers in reverse order of commits.
			commit := commits[len(commits)-i-1]

			if pathFilter.active() && !pathFilter.isInteresting(commit.TreeDiff) {
				ctl.DebugLog("skipping commit %q on %q because didn't match path filters", commit.Id, ref)
				continue
			}
			ctl.EmitTrigger(c, &internal.Trigger{
				Id:           fmt.Sprintf("%s/+/%s@%s", cfg.Repo, ref, commit.Id),
				Created:      commit.Committer.Time,
				OrderInBatch: int64(emittedTriggers),
				Title:        commit.Id,
				Url:          fmt.Sprintf("%s/+/%s", cfg.Repo, commit.Id),
				Payload: &internal.Trigger_Gitiles{
					Gitiles: &api.GitilesTrigger{Repo: cfg.Repo, Ref: ref, Revision: commit.Id},
				},
			})
			emittedTriggers++
		}
		// Stop early if next iteration can't emit maxCommitsPerRefUpdate triggers.
		// But do so only after first successful fetch to ensure progress if
		// misconfigured.
		if emittedTriggers+maxCommitsPerRefUpdate > maxTriggersPerInvocation {
			ctl.DebugLog("Emitted %d triggers, postponing the rest", emittedTriggers)
			return len(sortedRefs) - i - 1, nil
		}
	}
	return 0, nil
}

func (m TaskManager) getGitilesClient(c context.Context, ctl task.ControllerReadOnly, repo string) (*gitilesClient, error) {
	host, project, err := gitiles.ParseRepoURL(repo)
	if err != nil {
		return nil, errors.Fmt("invalid repo URL %q: %w", repo, err)
	}
	r := &gitilesClient{host: host, project: project}

	if m.mockGitilesClient != nil {
		// Used for testing only.
		logging.Infof(c, "using mockGitilesClient")
		r.GitilesClient = m.mockGitilesClient
		return r, nil
	}

	httpClient, err := ctl.GetClient(c, auth.WithScopes(scopes.GerritScopeSet()...))
	if err != nil {
		return nil, err
	}
	if r.GitilesClient, err = gitiles.NewRESTClient(httpClient, host, true); err != nil {
		return nil, err
	}
	return r, nil
}

// gitilesClient embeds GitilesClient with useful metadata.
type gitilesClient struct {
	gitilespb.GitilesClient
	host    string // Gitiles host
	project string // Gitiles project
}

type refsState struct {
	watched         gitiles.RefSet
	previousWatched gitiles.RefSet
	known           map[string]string // HEADs we saw before
	current         map[string]string // HEADs available now
	changed         int
}

func (s *refsState) pruneKnown(ctl task.Controller) {
	for ref := range s.known {
		switch {
		case !s.watched.Has(ref):
			ctl.DebugLog("Ref %s is no longer watched", ref)
			delete(s.known, ref)
			s.changed++
		case s.current[ref] == "":
			ctl.DebugLog("Ref %s deleted", ref)
			delete(s.known, ref)
			s.changed++
		}
	}
}

func (s *refsState) sortedCurrentRefNames() []string {
	sortedRefs := make([]string, 0, len(s.current))
	for ref := range s.current {
		sortedRefs = append(sortedRefs, ref)
	}
	sort.Strings(sortedRefs)
	return sortedRefs
}

// newCommits finds new commits for a given ref.
//
// If ref is new, returns only ref's HEAD,
// For updated refs, at most maxCommits of gitiles.Log(new..old)
func (s *refsState) newCommits(c context.Context, ctl task.Controller, g *gitilesClient, ref string, maxCommits int, pathFilter pathFilter) (
	commits []*git.Commit, newRef bool, err error) {
	newHead := s.current[ref]
	oldHead, existed := s.known[ref]
	switch {
	case !existed:
		ctl.DebugLog("Ref %s is new: %s", ref, newHead)
		maxCommits = 1
		newRef = true
	case oldHead != newHead:
		ctl.DebugLog("Ref %s updated: %s => %s", ref, oldHead, newHead)
	default:
		return // no change
	}

	c, cancel := clock.WithTimeout(c, gitilesRPCTimeout)
	defer cancel()

	commits, err = gitiles.PagingLog(c, g, &gitilespb.LogRequest{
		Project:            g.project,
		Committish:         newHead,
		ExcludeAncestorsOf: oldHead, // empty if ref is new, but then maxCommits is 1.
		PageSize:           int32(maxCommits),
		TreeDiff:           pathFilter.active(),
	}, maxCommits)
	switch status.Code(err) {
	case codes.OK:
		// Happy fast path.
		// len(commits) may be 0 if this ref had a force push reverting to some
		// older revision. TODO(tAndrii): consider emitting trigger with just
		// newHead commit if there is a compelling use case.
		s.known[ref] = newHead
		s.changed++
		return
	case codes.NotFound:
		// handled below.
		break
	default:
		// Any other error is presumably transient, so we'll retry.
		ctl.DebugLog("Ref %s: failed to fetch log between old %s and new %s revs", ref, oldHead, newHead)
		err = transient.Tag.Apply(err)
		return
	}
	// Either:
	//  (1) oldHead is no longer known in gitiles (force push),
	//  (2) newHead is no longer known in gitiles (eventual consistency,
	//     or concurrent force push executed just now, or ACLs change)
	//  (3) gitiles accidental 404, aka fluke.
	// In cases (2) and (3), retries should clear the problem, while (1) we
	// should handle now.
	if !existed {
		// There was no oldHead, so definitely not (1). Retry later.
		ctl.DebugLog("Ref %s: log of first rev %s not found", ref, newHead)
		err = transient.Tag.Apply(err)
		return
	}
	ctl.DebugLog("Ref %s: log old..new is not found, investigating further...", ref)

	// Fetch log of newHead only.
	var newErr error
	commits, newErr = gitiles.PagingLog(c, g, &gitilespb.LogRequest{
		Project:    g.project,
		Committish: newHead,
	}, 1)
	if newErr != nil {
		ctl.DebugLog("Ref %s: failed to fetch even log of just new rev %s %s", ref, newHead, err)
		err = transient.Tag.Apply(newErr)
		return
	}
	// Fetch log of oldHead only.
	_, errOld := gitiles.PagingLog(c, g, &gitilespb.LogRequest{
		Project:    g.project,
		Committish: oldHead,
		TreeDiff:   pathFilter.active(),
	}, 1)
	switch status.Code(errOld) {
	case codes.NotFound:
		// This is case (1). Since we've already fetched just 1 commit from
		// newHead, we are done.
		ctl.DebugLog("Ref %s: force push detected; emitting trigger for new head", ref)
		s.known[ref] = newHead
		s.changed++
		newRef = true
		err = nil
		return
	case codes.OK:
		ctl.DebugLog("Ref %s: weirdly, log(%s) and log(%s) work, but not log(%s..%s)",
			ref, oldHead, newHead, oldHead, newHead)
		err = transient.Tag.Apply(err)
		return
	default:
		// Any other error is presumably transient, so we'll retry.
		ctl.DebugLog("Ref %s: failed to fetch log of just old rev %s: %s", ref, oldHead, errOld)
		err = transient.Tag.Apply(err)
		return
	}
}

// pathFilter implements path_regexps[_exclude] filtering of commits.
type pathFilter struct {
	pathInclude *regexp.Regexp
	pathExclude *regexp.Regexp // must only be set iff pathInclude is. See also ValidateProtoMessage.
}

func newPathFilter(cfg *messages.GitilesTask) (p pathFilter, err error) {
	if len(cfg.PathRegexps) == 0 {
		return // PathRegexpsExclude are ignored in this case. See also ValidateProtoMessage.
	}
	if p.pathInclude, err = regexp.Compile(disjunctiveOfRegexps(cfg.PathRegexps)); err != nil {
		err = errors.Fmt("invalid path_regexps %q: %w", cfg.PathRegexps, err)
		return
	}
	if len(cfg.PathRegexpsExclude) > 0 {
		if p.pathExclude, err = regexp.Compile(disjunctiveOfRegexps(cfg.PathRegexpsExclude)); err != nil {
			err = errors.Fmt("invalid exclude_path_regexps %q: %w", cfg.PathRegexpsExclude, err)
			return
		}
	}
	return
}

func (p *pathFilter) active() bool {
	return p.pathInclude != nil
}

// isInteresting decides whether commit is interesting according to pathFilter
// based on which files were touched in commits.
func (p *pathFilter) isInteresting(diff []*git.Commit_TreeDiff) (skip bool) {
	isInterestingPath := func(path string) bool {
		switch {
		case path == "":
			return false
		case p.pathExclude != nil && p.pathExclude.MatchString(path):
			return false
		default:
			return p.pathInclude.MatchString(path)
		}
	}

	for _, d := range diff {
		if isInterestingPath(d.GetOldPath()) || isInterestingPath(d.GetNewPath()) {
			return true
		}
	}
	return false
}

func disjunctiveOfRegexps(rs []string) string {
	s := "^("
	for i, r := range rs {
		if i > 0 {
			s += "|"
		}
		s += "(" + r + ")"
	}
	s += ")$"
	return s
}
