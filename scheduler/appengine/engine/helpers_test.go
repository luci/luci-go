// Copyright 2018 The LUCI Authors.
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

package engine

// This file contains helpers used by the rest of tests.

import (
	"context"
	"errors"
	"math/rand"
	"sort"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/api/pubsub/v1"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"

	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
)

const fakeAppID = "scheduler-app-id"

var epoch = time.Unix(1442270520, 0).UTC()

func allJobs(c context.Context) []Job {
	datastore.GetTestable(c).CatchupIndexes()
	entities := []Job{}
	if err := datastore.GetAll(c, datastore.NewQuery("Job"), &entities); err != nil {
		panic(err)
	}
	// Strip UTC location pointers from zero time.Time{} so that ShouldResemble
	// can compare it to default time.Time{}. nil location is UTC too.
	for i := range entities {
		ent := &entities[i]
		if ent.Cron.LastRewind.IsZero() {
			ent.Cron.LastRewind = time.Time{}
		}
		if ent.Cron.LastTick.When.IsZero() {
			ent.Cron.LastTick.When = time.Time{}
		}
	}
	return entities
}

func sortedJobIds(jobs []*Job) []string {
	ids := stringset.New(len(jobs))
	for _, j := range jobs {
		ids.Add(j.JobID)
	}
	asSlice := ids.ToSlice()
	sort.Strings(asSlice)
	return asSlice
}

func newTestContext(now time.Time) context.Context {
	c := memory.UseWithAppID(context.Background(), fakeAppID)
	c = clock.Set(c, testclock.New(now))
	c = mathrand.Set(c, rand.New(rand.NewSource(1000)))
	c = secrets.Use(c, &testsecrets.Store{})

	// Signer is used by ShouldEnforceRealmACL to discover app ID.
	c = auth.ModifyConfig(c, func(cfg auth.Config) auth.Config {
		cfg.Signer = signingtest.NewSigner(&signing.ServiceInfo{
			AppID: fakeAppID,
		})
		return cfg
	})

	c, _, _ = tsmon.WithFakes(c)
	fake := store.NewInMemory(&target.Task{})
	tsmon.GetState(c).SetStore(fake)

	datastore.GetTestable(c).AddIndexes(&datastore.IndexDefinition{
		Kind: "Job",
		SortBy: []datastore.IndexColumn{
			{Property: "Enabled"},
			{Property: "ProjectID"},
		},
	})
	datastore.GetTestable(c).CatchupIndexes()

	return c
}

func newTestEngine() (*engineImpl, *fakeTaskManager) {
	mgr := &fakeTaskManager{}
	cat := catalog.New()
	cat.RegisterTaskManager(mgr)
	return NewEngine(Config{
		Catalog:        cat,
		Dispatcher:     &tq.Dispatcher{},
		PubSubPushPath: "/push-url",
	}).(*engineImpl), mgr
}

func mockOwnerCtx(ctx context.Context, realm string) context.Context {
	return auth.WithState(ctx, &authtest.FakeState{
		Identity: "user:owner@example.com",
		FakeDB: authtest.NewFakeDB(
			authtest.MockPermission("user:owner@example.com", realm, PermJobsGet),
			authtest.MockPermission("user:owner@example.com", realm, PermJobsPause),
			authtest.MockPermission("user:owner@example.com", realm, PermJobsResume),
			authtest.MockPermission("user:owner@example.com", realm, PermJobsAbort),
			authtest.MockPermission("user:owner@example.com", realm, PermJobsTrigger),
		),
	})
}

func mockReaderCtx(ctx context.Context, realm string) context.Context {
	return auth.WithState(ctx, &authtest.FakeState{
		Identity: "user:reader@example.com",
		FakeDB: authtest.NewFakeDB(
			authtest.MockPermission("user:reader@example.com", realm, PermJobsGet),
		),
	})
}

////

// fakeTaskManager implement task.Manager interface.
type fakeTaskManager struct {
	launchTask          func(ctx context.Context, ctl task.Controller) error
	abortTask           func(ctx context.Context, ctl task.Controller) error
	examineNotification func(ctx context.Context, msg *pubsub.PubsubMessage) string
	handleNotification  func(ctx context.Context, msg *pubsub.PubsubMessage) error
	handleTimer         func(ctx context.Context, ctl task.Controller, name string, payload []byte) error
}

func (m *fakeTaskManager) Name() string {
	return "fake"
}

func (m *fakeTaskManager) ProtoMessageType() proto.Message {
	return (*messages.NoopTask)(nil)
}

func (m *fakeTaskManager) Traits() task.Traits {
	return task.Traits{}
}

func (m *fakeTaskManager) ValidateProtoMessage(c *validation.Context, msg proto.Message, realmID string) {
}

func (m *fakeTaskManager) LaunchTask(c context.Context, ctl task.Controller) error {
	return m.launchTask(c, ctl)
}

func (m *fakeTaskManager) AbortTask(c context.Context, ctl task.Controller) error {
	if m.abortTask != nil {
		return m.abortTask(c, ctl)
	}
	return nil
}

func (m *fakeTaskManager) ExamineNotification(c context.Context, msg *pubsub.PubsubMessage) string {
	return m.examineNotification(c, msg)
}

func (m *fakeTaskManager) HandleNotification(c context.Context, ctl task.Controller, msg *pubsub.PubsubMessage) error {
	return m.handleNotification(c, msg)
}

func (m fakeTaskManager) HandleTimer(c context.Context, ctl task.Controller, name string, payload []byte) error {
	return m.handleTimer(c, ctl, name, payload)
}

func (m fakeTaskManager) GetDebugState(c context.Context, ctl task.ControllerReadOnly) (*internal.DebugManagerState, error) {
	return nil, errors.New("not implemented")
}
