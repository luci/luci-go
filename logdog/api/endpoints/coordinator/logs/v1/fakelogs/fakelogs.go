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

// Package fakelogs implements a fake LogClient for use in tests.
package fakelogs

import (
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"
	logs_service "go.chromium.org/luci/logdog/appengine/coordinator/endpoints/logs"
	reg_service "go.chromium.org/luci/logdog/appengine/coordinator/endpoints/registration"
	srv_service "go.chromium.org/luci/logdog/appengine/coordinator/endpoints/services"
	mem_storage "go.chromium.org/luci/logdog/common/storage/memory"
)

// NewClient generates a new fake Client which can be used as a logs.LogsClient,
// and can also have its underlying stream data manipulated by the test.
//
// Functions taking context.Context will ignore it (i.e. they don't expect
// anything in the context).
//
// This client is preconfiguerd
func NewClient() *Client {
	ctx, env := coordinatorTest.Install()
	storage := mem_storage.Storage{}
	env.Services.ST = func(*coordinator.LogStreamState) (coordinator.Storage, error) {
		return &storage, nil
	}
	return &Client{
		ctx: ctx, env: env,

		logsServ: logs_service.New(),
		regServ:  reg_service.New(),
		srvServ:  srv_service.New(),

		storage: &storage,
	}
}
