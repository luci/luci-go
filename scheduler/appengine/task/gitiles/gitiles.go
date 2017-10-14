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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/api/pubsub/v1"

	ds "go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/server/auth"
)

// defaultMaxTriggersPerInvocation limits number of triggers emitted per one
// invocation.
const defaultMaxTriggersPerInvocation = 100

// TaskManager implements task.Manager interface for tasks defined with
// GitilesTask proto message.
type TaskManager struct {
	mockGitilesClient        gitilesClient // Used for testing only.
	maxTriggersPerInvocation int           // Avoid choking on DS or runtime limits.
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
func (m TaskManager) ValidateProtoMessage(msg proto.Message) error {
	cfg, ok := msg.(*messages.GitilesTask)
	if !ok {
		return fmt.Errorf("wrong type %T, expecting *messages.GitilesTask", msg)
	}

	// Validate 'repo' field.
	if cfg.Repo == "" {
		return fmt.Errorf("field 'repository' is required")
	}
	u, err := url.Parse(cfg.Repo)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %s", cfg.Repo, err)
	}
	if !u.IsAbs() {
		return fmt.Errorf("not an absolute url: %q", cfg.Repo)
	}
	for _, ref := range cfg.Refs {
		if !strings.HasPrefix(ref, "refs/") {
			return fmt.Errorf("ref must start with 'refs/' not %q", ref)
		}
		cnt := strings.Count(ref, "*")
		if cnt > 1 || (cnt == 1 && !strings.HasSuffix(ref, "/*")) {
			return fmt.Errorf("only trailing (e.g. refs/blah/*) globs are supported, not %q", ref)
		}
	}
	return nil
}

// LaunchTask is part of Manager interface.
func (m TaskManager) LaunchTask(c context.Context, ctl task.Controller, triggers []*internal.Trigger) error {
	cfg := ctl.Task().(*messages.GitilesTask)

	ctl.DebugLog("Repo: %s, Refs: %s", cfg.Repo, cfg.Refs)
	u, err := url.Parse(cfg.Repo)
	if err != nil {
		return err
	}

	watchedRefs := watchedRefs{}
	watchedRefs.init(cfg.GetRefs())

	var wg sync.WaitGroup

	var heads map[string]string
	var headsErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		heads, headsErr = m.load(c, ctl.JobID(), u)
	}()

	var refs map[string]string
	var refsErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		refs, refsErr = m.getRefsTips(c, ctl, cfg.Repo, watchedRefs)
	}()

	wg.Wait()

	if headsErr != nil {
		ctl.DebugLog("Failed to fetch heads - %s", headsErr)
		return fmt.Errorf("failed to fetch heads: %v", headsErr)
	}
	if refsErr != nil {
		ctl.DebugLog("Failed to fetch refs - %s", refsErr)
		return fmt.Errorf("failed to fetch refs: %v", refsErr)
	}

	refsChanged := 0

	// Delete all previously known refs which are either no longer watched or no
	// longer exist in repo.
	for ref := range heads {
		switch {
		case !watchedRefs.hasRef(ref):
			ctl.DebugLog("Ref %s is no longer watched", ref)
			delete(heads, ref)
			refsChanged++
		case refs[ref] == "":
			ctl.DebugLog("Ref %s deleted", ref)
			delete(heads, ref)
			refsChanged++
		}
	}
	// For determinism, sort keys of current refs.
	sortedRefs := make([]string, 0, len(refs))
	for ref := range refs {
		sortedRefs = append(sortedRefs, ref)
	}
	sort.Strings(sortedRefs)

	emittedTriggers := 0
	maxTriggersPerInvocation := m.maxTriggersPerInvocation
	if maxTriggersPerInvocation == 0 {
		maxTriggersPerInvocation = defaultMaxTriggersPerInvocation
	}
	// Note, that current `refs` contain only watched refs (see getRefsTips).
	for _, ref := range sortedRefs {
		newHead := refs[ref]
		oldHead, existed := heads[ref]
		switch {
		case !existed:
			ctl.DebugLog("Ref %s is new: %s", ref, newHead)
		case oldHead != newHead:
			ctl.DebugLog("Ref %s updated: %s => %s", ref, oldHead, newHead)
		default:
			// No change.
			continue
		}
		heads[ref] = newHead
		refsChanged++
		emittedTriggers++
		// TODO(tandrii): actually look at commits between current and previously
		// known tips of each ref.
		// In current (v1) engine, all triggers emitted around the same time will
		// result in just 1 invocation of each triggered job. Therefore,
		// passing just HEAD's revision is good enough.
		// For the same reason, only 1 of the refs will actually be processed if
		// several refs changed at the same time.
		ctl.EmitTrigger(c, &internal.Trigger{
			Id:    fmt.Sprintf("%s/+/%s@%s", cfg.Repo, ref, newHead),
			Title: newHead,
			Url:   fmt.Sprintf("%s/+/%s", cfg.Repo, newHead),
			Payload: &internal.Trigger_Gitiles{
				Gitiles: &internal.GitilesTriggerData{Repo: cfg.Repo, Ref: ref, Revision: newHead},
			},
		})

		// Safeguard against too many changes such as the first run after
		// config change to watch many more refs than before.
		if emittedTriggers >= maxTriggersPerInvocation {
			ctl.DebugLog("Emitted %d triggers, postponing the rest", emittedTriggers)
			break
		}
	}

	if refsChanged == 0 {
		ctl.DebugLog("No changes detected")
	} else {
		ctl.DebugLog("%d refs changed", refsChanged)
		// Force save to ensure triggers are actually emitted.
		if err := ctl.Save(c); err != nil {
			// At this point, triggers have not been sent, so bail now and don't save
			// the refs' heads newest values.
			return err
		}
		if err := m.save(c, ctl.JobID(), u, heads); err != nil {
			return err
		}
		ctl.DebugLog("Saved %d known refs", len(heads))
	}

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

// Reference is used to store the revision of a ref.
type Reference struct {
	// Name is the reference name.
	Name string `gae:",noindex"`

	// Revision is the ref commit.
	Revision string `gae:",noindex"`
}

// Repository is used to store the repository status.
type Repository struct {
	_kind  string         `gae:"$kind,gitiles.Repository"`
	_extra ds.PropertyMap `gae:"-,extra"`

	// ID is "<job ID>:<repository URL>".
	ID string `gae:"$id"`

	// References is the slice of all the tracked refs within repository.
	References []Reference `gae:",noindex"`
}

func repositoryID(jobID string, u *url.URL) string {
	return fmt.Sprintf("%s:%s", jobID, u)
}

func (m TaskManager) load(c context.Context, jobID string, u *url.URL) (map[string]string, error) {
	stored := &Repository{ID: repositoryID(jobID, u)}
	err := ds.Get(c, stored)
	if err != nil && err != ds.ErrNoSuchEntity {
		return nil, err
	}
	heads := make(map[string]string, len(stored.References))
	for _, b := range stored.References {
		heads[b.Name] = b.Revision
	}
	return heads, nil
}

func (m TaskManager) save(c context.Context, jobID string, u *url.URL, heads map[string]string) error {
	sortedRefs := make([]string, 0, len(heads))
	for ref := range heads {
		sortedRefs = append(sortedRefs, ref)
	}
	sort.Strings(sortedRefs)

	refs := make([]Reference, 0, len(heads))
	for _, n := range sortedRefs {
		refs = append(refs, Reference{
			Name:     n,
			Revision: heads[n],
		})
	}
	return transient.Tag.Apply(ds.Put(c, &Repository{
		ID:         repositoryID(jobID, u),
		References: refs,
	}))
}

// getRefsTips returns tip for each ref being watched.
func (m TaskManager) getRefsTips(c context.Context, ctl task.Controller, repo string, watched watchedRefs) (map[string]string, error) {
	g, err := m.getGitilesClient(c, ctl)
	if err != nil {
		return nil, err
	}

	// Query gitiles for each namespace in parallel.
	var wg sync.WaitGroup
	var lock sync.Mutex
	errs := []error{}
	allTips := map[string]string{}
	// Group all refs by their namespace to reduce # of RPCs.
	for _, wrs := range watched.namespaces {
		wg.Add(1)
		go func(wrs *watchedRefNamespace) {
			defer wg.Done()
			tips, err := g.Refs(c, repo, wrs.namespace)
			lock.Lock()
			defer lock.Unlock()
			if err != nil {
				ctl.DebugLog("failed to fetch %q namespace tips for %q: %q", wrs.namespace, err)
				errs = append(errs, err)
				return
			}
			for ref, tip := range tips {
				if watched.hasRef(ref) {
					allTips[ref] = tip
				}
			}
		}(wrs)
	}
	wg.Wait()
	if len(errs) > 0 {
		return nil, errors.NewMultiError(errs...)
	}
	return allTips, nil
}

type gitilesClient interface {
	Refs(ctx context.Context, repoURL, refsPath string) (map[string]string, error)
}

func (m TaskManager) getGitilesClient(c context.Context, ctl task.Controller) (gitilesClient, error) {
	httpClient, err := ctl.GetClient(c, time.Minute, auth.WithScopes(
		"https://www.googleapis.com/auth/gerritcodereview",
	))
	if err != nil {
		return nil, err
	}
	if m.mockGitilesClient != nil {
		// Used for testing only.
		logging.Infof(c, "using mockGitilesClient")
		return m.mockGitilesClient, nil
	}
	return &gitiles.Client{Client: httpClient}, nil
}

func splitRef(s string) (string, string) {
	if i := strings.LastIndex(s, "/"); i <= 0 {
		return s, ""
	} else {
		return s[:i], s[i+1:]
	}
}

type watchedRefNamespace struct {
	namespace    string // no trailing "/".
	allChildren  bool   // if true, someChildren is ignored.
	someChildren stringset.Set
}

func (w watchedRefNamespace) hasSuffix(suffix string) bool {
	switch {
	case suffix == "*":
		panic(fmt.Errorf("watchedRefNamespace membership should only be checked for refs, not ref glob %s", suffix))
	case w.allChildren:
		return true
	case w.someChildren == nil:
		return false
	default:
		return w.someChildren.Has(suffix)
	}
}

func (w *watchedRefNamespace) addSuffix(suffix string) {
	switch {
	case w.allChildren:
		return
	case suffix == "*":
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

func (w *watchedRefs) init(refsConfig []string) {
	w.namespaces = map[string]*watchedRefNamespace{}
	for _, ref := range refsConfig {
		ns, suffix := splitRef(ref)
		if _, exists := w.namespaces[ns]; !exists {
			w.namespaces[ns] = &watchedRefNamespace{namespace: ns}
		}
		w.namespaces[ns].addSuffix(suffix)
	}
}

func (w *watchedRefs) hasRef(ref string) bool {
	ns, suffix := splitRef(ref)
	if wrn, exists := w.namespaces[ns]; exists {
		return wrn.hasSuffix(suffix)
	}
	return false
}
