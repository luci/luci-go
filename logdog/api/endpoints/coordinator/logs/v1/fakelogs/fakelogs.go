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
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"
	logs_service "go.chromium.org/luci/logdog/appengine/coordinator/endpoints/logs"
	reg_service "go.chromium.org/luci/logdog/appengine/coordinator/endpoints/registration"
	srv_service "go.chromium.org/luci/logdog/appengine/coordinator/endpoints/services"
	mem_storage "go.chromium.org/luci/logdog/common/storage/memory"
)

type storageclient struct {
	*mem_storage.Storage
}

// Close is implemented here so that we can return this coordinator.Storage
// client multiple times without worrying about the coordinator closing it.
func (s storageclient) Close() {}

// GetSignedURLs is implemented to fill out the coordinator.Storage interface.
func (s storageclient) GetSignedURLs(context.Context, *coordinator.URLSigningRequest) (*coordinator.URLSigningResponse, error) {
	return nil, errors.New("NOT IMPLEMENTED")
}

// NewClient generates a new fake Client which can be used as a logs.LogsClient,
// and can also have its underlying stream data manipulated by the test.
//
// Functions taking context.Context will ignore it (i.e. they don't expect
// anything in the context).
//
// This client is attached to an in-memory datastore of its own and the real
// coordinator services are attached to that in-memory datastore. That means
// that the Client API SHOULD actually behave identically to the real
// coordinator (since it's the same code). Additionally, the 'Open' methods on
// the Client do the full Prefix/Stream registration process, and so should also
// behave like the real thing.
func NewClient() *Client {
	ctx, env := coordinatorTest.Install(false)
	env.LogIn()
	env.AuthState.IdentityGroups = []string{"admin", "all", "auth", "services"}

	storage := &mem_storage.Storage{}
	env.Services.ST = func(*coordinator.LogStreamState) (coordinator.Storage, error) {
		return storageclient{storage}, nil
	}
	return &Client{
		ctx: ctx, env: env,

		logsServ: logs_service.New(),
		regServ:  reg_service.New(),
		srvServ:  srv_service.New(),

		storage: storage,

		prefixes: map[string]*prefixState{},
	}
}
