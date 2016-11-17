// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package gitiles

import (
	"fmt"
	"net/url"
	"sort"
	"sync"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/api/pubsub/v1"

	ds "github.com/luci/gae/service/datastore"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/scheduler/appengine/messages"
	"github.com/luci/luci-go/scheduler/appengine/task"
	"github.com/luci/luci-go/scheduler/appengine/task/gitiles/gerrit"
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
func (m TaskManager) LaunchTask(c context.Context, ctl task.Controller) error {
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
		heads, headsErr = m.load(c, u)
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
	if err := m.save(c, u, heads); err != nil {
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

	// Repository is the repository URL.
	Repository string `gae:"$id"`

	// References is the slice of all the tracked refs within repository.
	References []Reference `gae:",noindex"`
}

func (m TaskManager) load(c context.Context, u *url.URL) (map[string]string, error) {
	stored := &Repository{Repository: u.String()}
	err := ds.Get(c, stored)
	if err != nil && err != ds.ErrNoSuchEntity {
		return nil, err
	}
	heads := map[string]string{}
	for _, b := range stored.References {
		heads[b.Name] = b.Revision
	}
	return heads, nil
}

func (m TaskManager) save(c context.Context, u *url.URL, heads map[string]string) error {
	stored := &Repository{Repository: u.String()}
	err := ds.Get(c, stored)
	if err != nil && err != ds.ErrNoSuchEntity {
		return err
	}
	refs := []Reference{}
	for n, r := range heads {
		refs = append(refs, Reference{
			Name:     n,
			Revision: r,
		})
	}
	stored.References = refs
	return ds.Put(c, stored)
}
