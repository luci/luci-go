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

// Package ttqgaeds implements task enqueueing into Cloud Tasks with
// transactional semantics from inside a Cloud Datastore transaction
// running on AppEngine (v1 or v2).
//
// Limitations:
//   * Does NOT support named tasks, for which Cloud Tasks provides
//     de-duplication. This is also a limitation of AppEngine Classic
//     transactional Tasks enqueueing.
//     Therefore, if you need to de-duplicate, you need to do this in your
//     application code yourself.
//   * All limits of Cloud Tasks apply, see
//     https://cloud.google.com/tasks/docs/quotas.
//
// Although the package depends on "go.chromium.org/gae/service/datastore",
// it requires only regular Cloud APIs and works from anywhere (not necessarily
// from Appengine). See also
// https://godoc.org/go.chromium.org/luci/server/gaeemulation
// and
// https://godoc.org/go.chromium.org/gae/impl/cloud.
package ttqgaeds

import (
	"context"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"

	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/ttq"
	"go.chromium.org/luci/ttq/internal"
	"go.chromium.org/luci/ttq/internal/dbds"
	"go.chromium.org/luci/ttq/internal/sweepdrivergae"
)

// TTQ provides transaction task enqueueing with Datastore backend on AppEngine.
//
// Designed to be the least disruptive replacement for transaction task enqueing
// on classic AppEngine.
type TTQ struct {
	impl internal.Impl
}

// New creates a new TTQ for Datastore on AppEngine.
// You must also call SetupSweeping in at least one of your app's microservices.
func New(c *cloudtasks.Client, opts ttq.Options) (*TTQ, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	return &TTQ{impl: internal.Impl{
		Options:     opts,
		DB:          &dbds.DB{},
		TasksClient: internal.UnborkTasksClient(c),
	}}, nil
}

// SetupSweeping establishes routes for sweeping.
// Please, read sweepdrivergae.Factory
//     https://go.chromium.org/luci/ttq/internal/sweepdriversgae/#Factory
// documentation for one time setup and the meaning of arguments.
//
// Panics if called twice.
func (t *TTQ) SetupSweeping(r *router.Router, mw router.MiddlewareChain,
	pathPrefix, queue string) {
	t.impl.SetupSweeping(sweepdrivergae.Factory(r, mw, pathPrefix, queue, t.impl.TasksClient))
}

// AddTask guarantees eventual creation of a task in Cloud Tasks if the current
// transaction completes successfully.
//
// The returned ttq.PostProcess should be called after the successful
// transaction. See ttq.PostProcess
//     https://godoc.org/go.chromium.org/luci/ttq#PostProcess
// documentation for more info.
//
// Panics if not called with a transaction context.
func (t *TTQ) AddTask(ctx context.Context, req *taskspb.CreateTaskRequest) (ttq.PostProcess, error) {
	return t.impl.AddTask(ctx, req)
}
