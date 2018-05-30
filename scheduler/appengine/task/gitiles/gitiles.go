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
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/api/pubsub/v1"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/auth"

	api "go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
)

// defaultMaxTriggersPerInvocation limits number of triggers emitted per one
// invocation.
const defaultMaxTriggersPerInvocation = 100

// defaultMaxCommitsPerRefUpdate limits number of commits (and hence triggers)
// emitted when a ref changes.
// Must be smaller than defaultMaxTriggersPerInvocation, else these many
// triggers could be emitted.
const defaultMaxCommitsPerRefUpdate = 50

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

func validateRegexpRef(c *validation.Context, ref string) {
	c.Enter(ref)
	defer c.Exit()
	reStr := strings.TrimPrefix(ref, "regexp:")
	if strings.HasPrefix(reStr, "^") || strings.HasSuffix(reStr, "$") {
		c.Errorf("regexp ^ and $ qualifiers are added automatically, " +
			"please remove them from the config")
		return
	}
	r, err := regexp.Compile(reStr)
	if err != nil {
		c.Errorf("invalid regexp: %s", err)
		return
	}
	lp, complete := r.LiteralPrefix()
	if complete {
		c.Errorf("matches a single ref only, please use %q instead", lp)
	}
	if !strings.HasPrefix(reStr, lp) {
		c.Errorf("does not start with its own literal prefix %q", lp)
	}
	if strings.Count(lp, "/") < 2 {
		c.Errorf(`fewer than 2 slashes in literal prefix %q, e.g., `+
			`"refs/heads/\d+" is accepted because of "refs/heads/" is the `+
			`literal prefix, while "refs/.*" is too short`, lp)
	}
	if !strings.HasPrefix(lp, "refs/") {
		c.Errorf(`literal prefix %q must start with "refs/"`, lp)
	}
}

// ValidateProtoMessage is part of Manager interface.
func (m TaskManager) ValidateProtoMessage(c *validation.Context, msg proto.Message) {
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
	for _, ref := range cfg.Refs {
		if strings.HasPrefix(ref, "regexp:") {
			validateRegexpRef(c, ref)
			continue
		}

		if !strings.HasPrefix(ref, "refs/") {
			c.Errorf("ref must start with 'refs/' not %q", ref)
		}
		cnt := strings.Count(ref, "*")
		if cnt > 1 || (cnt == 1 && !strings.HasSuffix(ref, "/*")) {
			c.Errorf("only trailing (e.g. refs/blah/*) globs are supported, not %q", ref)
		}
	}
	c.Exit()
}

// LaunchTask is part of Manager interface.
func (m TaskManager) LaunchTask(c context.Context, ctl task.Controller) error {
	cfg := ctl.Task().(*messages.GitilesTask)
	ctl.DebugLog("Repo: %s, Refs: %s", cfg.Repo, cfg.Refs)

	g, err := m.getGitilesClient(c, ctl, cfg)
	if err != nil {
		return err
	}
	// TODO(tandrii): use g.host, g.project for saving/loading state
	// instead of repoURL.
	repoURL, err := url.Parse(cfg.Repo)
	if err != nil {
		return err
	}

	refs, err := m.fetchRefsState(c, ctl, cfg, g, repoURL)
	if err != nil {
		ctl.DebugLog("Error fetching state of the world: %s", err)
		return err
	}

	refs.pruneKnown(ctl)
	leftToProcess, err := m.emitTriggersRefAtATime(c, ctl, g, cfg.Repo, refs)

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
	if err := saveState(c, ctl.JobID(), repoURL, refs.known); err != nil {
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

// HandleNotification is part of Manager interface.
func (m TaskManager) HandleNotification(c context.Context, ctl task.Controller, msg *pubsub.PubsubMessage) error {
	return errors.New("not implemented")
}

// HandleTimer is part of Manager interface.
func (m TaskManager) HandleTimer(c context.Context, ctl task.Controller, name string, payload []byte) error {
	return errors.New("not implemented")
}

func (m TaskManager) fetchRefsState(c context.Context, ctl task.Controller, cfg *messages.GitilesTask, g *gitilesClient, repoURL *url.URL) (*refsState, error) {
	refs := &refsState{}
	refs.watched.init(c, cfg.GetRefs())
	return refs, parallel.FanOutIn(func(work chan<- func() error) {
		work <- func() (loadErr error) {
			refs.known, loadErr = loadState(c, ctl.JobID(), repoURL)
			return
		}
		// Group all refs by their namespace to reduce # of RPCs to gitiles.
		for _, wrs := range refs.watched.namespaces {
			wrs := wrs
			work <- func() error {
				return refs.fetchCurrentTips(c, wrs.namespace, g)
			}
		}
	})
}

// emitTriggersRefAtATime processes refs one a time and emits triggers if ref
// changed. Limits number of triggers emitted and so may stop early.
//
// Returns how many refs were not examined.
func (m TaskManager) emitTriggersRefAtATime(c context.Context, ctl task.Controller, g *gitilesClient, repo string, refs *refsState) (int, error) {
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
	for i, ref := range sortedRefs {
		commits, err := refs.newCommits(c, ctl, g, ref, maxCommitsPerRefUpdate)
		if err != nil {
			// This ref counts as not yet examined.
			return len(sortedRefs) - i, err
		}
		for i := range commits {
			// commit[0] is latest, so emit triggers in reverse order of commits.
			commit := commits[len(commits)-i-1]
			ctl.EmitTrigger(c, &internal.Trigger{
				Id:           fmt.Sprintf("%s/+/%s@%s", repo, ref, commit.Id),
				Created:      commit.Committer.Time,
				OrderInBatch: int64(emittedTriggers),
				Title:        commit.Id,
				Url:          fmt.Sprintf("%s/+/%s", repo, commit.Id),
				Payload: &internal.Trigger_Gitiles{
					Gitiles: &api.GitilesTrigger{Repo: repo, Ref: ref, Revision: commit.Id},
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

func (m TaskManager) getGitilesClient(c context.Context, ctl task.Controller, cfg *messages.GitilesTask) (*gitilesClient, error) {
	host, project, err := gitiles.ParseRepoURL(cfg.Repo)
	if err != nil {
		return nil, errors.Annotate(err, "invalid repo URL %q", cfg.Repo).Err()
	}
	r := &gitilesClient{host: host, project: project}

	if m.mockGitilesClient != nil {
		// Used for testing only.
		logging.Infof(c, "using mockGitilesClient")
		r.GitilesClient = m.mockGitilesClient
		return r, nil
	}

	httpClient, err := ctl.GetClient(c, time.Minute, auth.WithScopes(gitiles.OAuthScope))
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
	watched     watchedRefs
	known       map[string]string // HEADs we saw before
	current     map[string]string // HEADs available now
	changed     int
	currentLock sync.Mutex // Used because fetching current is in parallel
}

func (s *refsState) fetchCurrentTips(c context.Context, refsPath string, g *gitilesClient) error {
	resp, err := g.Refs(c, &gitilespb.RefsRequest{
		Project:  g.project,
		RefsPath: refsPath,
	})
	if err != nil {
		return err
	}
	s.currentLock.Lock()
	defer s.currentLock.Unlock()
	if s.current == nil {
		s.current = map[string]string{}
	}
	for ref, tip := range resp.Revisions {
		if s.watched.hasRef(ref) {
			s.current[ref] = tip
		}
	}
	return nil
}

func (s *refsState) pruneKnown(ctl task.Controller) {
	for ref := range s.known {
		switch {
		case !s.watched.hasRef(ref):
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
func (s *refsState) newCommits(c context.Context, ctl task.Controller, g *gitilesClient, ref string, maxCommits int) ([]*git.Commit, error) {
	newHead := s.current[ref]
	oldHead, existed := s.known[ref]
	switch {
	case !existed:
		ctl.DebugLog("Ref %s is new: %s", ref, newHead)
		maxCommits = 1
	case oldHead != newHead:
		ctl.DebugLog("Ref %s updated: %s => %s", ref, oldHead, newHead)
	default:
		return nil, nil // no change
	}

	commits, err := gitiles.PagingLog(c, g, gitilespb.LogRequest{
		Project:  g.project,
		Treeish:  newHead,
		Ancestor: oldHead, // empty if ref is new, but then maxCommits is 1.
		PageSize: int32(maxCommits),
	}, maxCommits)
	if err != nil {
		ctl.DebugLog("Failed to fetch log of new rev %q", newHead)
		return nil, err
	}

	s.known[ref] = newHead
	s.changed++
	return commits, nil
}

type watchedRefNamespace struct {
	namespace string // no trailing "/".
	// TODO(sergiyb): remove allChildren while removing support of ref globs after
	// all configs using them are updated to use regexp.
	allChildren bool // if true, someChildren is ignored.
	// someChildren is a set of immediate children, not grandchildren. i.e., may
	// contain 'child', but not 'grand/child', which would be contained in
	// refsNamespace of (namespace + "/child").
	someChildren stringset.Set
	// descendantRegexp is a regular expression matching all descendants.
	descendantRegexp *regexp.Regexp
}

func (w watchedRefNamespace) hasSuffix(suffix string) bool {
	switch {
	case suffix == "*":
		panic(fmt.Errorf("watchedRefNamespace membership should only be checked for refs, not ref glob %s", suffix))
	case w.allChildren && !strings.Contains(suffix, "/"):
		return true
	case w.descendantRegexp != nil && w.descendantRegexp.MatchString(suffix):
		return true
	case w.someChildren == nil:
		return false
	default:
		return w.someChildren.Has(suffix)
	}
}

func (w *watchedRefNamespace) addSuffix(c context.Context, suffix string) {
	switch {
	case w.allChildren:
		return
	case suffix == "*":
		logging.Warningf(c, "globs are deprecated, please update configs to use regexp instead")
		w.allChildren = true
		w.someChildren = nil
		return
	case w.someChildren == nil:
		w.someChildren = stringset.New(1)
	}
	w.someChildren.Add(suffix)
}

type watchedRefs struct {
	namespaces map[string]*watchedRefNamespace
}

func parseRefFromConfig(ref string) (ns, literalSuffix, regexpSuffix string) {
	if strings.HasPrefix(ref, "regexp:") {
		ref = strings.TrimPrefix(ref, "regexp:")
		descendantRegexp, err := regexp.Compile(ref)
		if err != nil {
			panic(err)
		}
		literalPrefix, _ := descendantRegexp.LiteralPrefix()
		ns = literalPrefix[:strings.LastIndex(literalPrefix, "/")]
		regexpSuffix = ref[len(ns)+1:]
		return
	}

	ns, literalSuffix = splitRef(ref)
	return
}

func (w *watchedRefs) init(c context.Context, refsConfig []string) {
	w.namespaces = map[string]*watchedRefNamespace{}
	nsRegexps := map[string][]string{}
	for _, ref := range refsConfig {
		ns, suffix, regexp := parseRefFromConfig(ref)
		if _, exists := w.namespaces[ns]; !exists {
			w.namespaces[ns] = &watchedRefNamespace{namespace: ns}
		}

		switch {
		case (suffix == "") == (regexp == ""):
			panic("exactly one must be defined")
		case regexp != "":
			nsRegexps[ns] = append(nsRegexps[ns], regexp)
		case suffix != "":
			w.namespaces[ns].addSuffix(c, suffix)
		}
	}

	for ns, regexps := range nsRegexps {
		var err error
		w.namespaces[ns].descendantRegexp, err = regexp.Compile(
			"^(" + strings.Join(regexps, ")|(") + ")$")
		if err != nil {
			panic(err)
		}
	}
}

func (w *watchedRefs) hasRef(ref string) bool {
	for ns, wrn := range w.namespaces {
		nsPrefix := ns + "/"
		if strings.HasPrefix(ref, nsPrefix) && wrn.hasSuffix(ref[len(nsPrefix):]) {
			return true
		}
	}

	return false
}

func splitRef(s string) (string, string) {
	i := strings.LastIndex(s, "/")
	if i <= 0 {
		return s, ""
	}
	return s[:i], s[i+1:]
}
