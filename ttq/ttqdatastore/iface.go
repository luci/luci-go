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

// Package ttqdatastore implements task enqueueing into Cloud Tasks with
// transactional semantics from inside a Cloud Datastore transaction.
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
package ttqdatastore

import (
	"context"
	"errors"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"

	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/ttq"
	"go.chromium.org/luci/ttq/internal"
)

// TTQ implements transaction task enqueueing with Datastore backend.
type TTQ struct {
	impl internal.Impl
}

// New creates a new TTQ for Datastore.
// You must also call InstallRoutes in at least one of your app's microservices.
func New(c *cloudtasks.Client, opts ttq.Options) (*TTQ, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	return &TTQ{impl: internal.Impl{
		Options:     opts,
		DB:          &db{},
		TasksClient: c,
	}}, nil
}

// InstallRoutes installs handlers for sweeping to ensure correctness.
//
// Users must ensure at least one of their microservices calls InstallRoutes.
//
// Requires a ttq.Options.Queue to be available.
// Reserves a ttq.Options.BaseURL path component in the given router for its own
// use.
// Do read the documentation about the required cron setup in ttq.Options:
//   TODO(tandrii): link
//
// Panics if called twice.
func (t *TTQ) InstallRoutes(r *router.Router, mw router.MiddlewareChain) {
	// TODO(tandrii): implement.
}

// AddTask guarantees eventual creation of a task in Cloud Tasks if the current
// transaction completes successfully.
//
// The returned ttq.PostProcess should be called after the successful
// transaction. See ttq.PostProcess
//   TODO(tandrii): link
// documentation for more info.
//
// Panics if not called with a transaction context.
func (t *TTQ) AddTask(ctx context.Context, req *taskspb.CreateTaskRequest) (ttq.PostProcess, error) {
	// TODO(tandrii): implement.
	return nil, errors.New("not implemented")
}
