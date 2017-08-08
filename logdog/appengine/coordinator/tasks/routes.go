// Copyright 2017 The LUCI Authors.
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

package tasks

import (
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/server/router"
)

const (
	// ArchivalTaskQueue is the name of the archival task queue.
	ArchivalTaskQueue = "archival"
)

// Dispatcher is the task dispatcher that handles all tasks in this package.
var taskDispatcher tq.Dispatcher

// RegisterTasks registers all supported tasks with the supplied task queue
// Dispatcher.
func init() {
	taskDispatcher.RegisterTask(&logdog.ArchiveDispatchTask{}, handleArchiveDispatchTask, ArchivalTaskQueue, nil)
}

// InstallHandlers registers task processing handlers into the supplied router.
func InstallHandlers(r *router.Router, base router.MiddlewareChain) {
	taskDispatcher.InstallRoutes(r, base)
}
