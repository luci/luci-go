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
	"sync"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/api/pubsub/v1"

	ds "go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/gitiles/gerrit"
)

// TaskManager implements task.Manager interface for tasks defined with
// SwarmingTask proto message.
type TaskManager struct {
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

	return nil
}

// LaunchTask is part of Manager interface.
func (m TaskManager) LaunchTask(c context.Context, ctl task.Controller, triggers []task.Trigger) error {
	cfg := ctl.Task().(*messages.GitilesTask)

	u, err := url.Parse(cfg.Repo)
	if err != nil {
		return err
	}

	g := gerrit.NewGerrit(u)

	var wg sync.WaitGroup

	var heads map[string]string
	var headsErr error
	wg.Add(1)
	go func() {
		heads, headsErr = m.load(c, ctl.JobID(), u)
		wg.Done()
	}()

	var refs map[string]string
	var refsErr error
	wg.Add(1)
	go func() {
		refs, refsErr = g.GetRefs(c, cfg.Refs)
		wg.Done()
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

	type result struct {
		ref string
		log gerrit.Log
		err error
	}
	ch := make(chan result)

	for ref, head := range refs {
		if val, ok := heads[ref]; !ok {
			wg.Add(1)
			go func(ref string) {
				l, err := g.GetLog(c, ref, fmt.Sprintf("%s~", ref))
				ch <- result{ref, l, err}
				wg.Done()
			}(ref)
		} else if val != head {
			wg.Add(1)
			go func(ref, since string) {
				l, err := g.GetLog(c, ref, since)
				ch <- result{ref, l, err}
				wg.Done()
			}(ref, val)
		}
	}
	go func() {
		wg.Wait()
		close(ch)
	}()

	var log gerrit.Log
	for r := range ch {
		if r.err != nil {
			ctl.DebugLog("Failed to fetch log - %s", r.err)
			if gerrit.IsNotFound(r.err) {
				delete(heads, r.ref)
			} else {
				return r.err
			}
		}
		if len(r.log) > 0 {
			heads[r.ref] = r.log[len(r.log)-1].Commit
		}
		log = append(log, r.log...)
	}

	sort.Sort(log)
	for _, commit := range log {
		// TODO(phosek): Trigger buildbucket job here.
		ctl.DebugLog("Trigger build for commit %s", commit.Commit)
	}
	if err := m.save(c, ctl.JobID(), u, heads); err != nil {
		return err
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
	refs := make([]Reference, 0, len(heads))
	for n, r := range heads {
		refs = append(refs, Reference{
			Name:     n,
			Revision: r,
		})
	}
	return ds.Put(c, &Repository{
		ID:         repositoryID(jobID, u),
		References: refs,
	})
}
