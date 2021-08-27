// Copyright 2021 The LUCI Authors.
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

// Package impl contains code shared by `frontend` and `backend` services.
package impl

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/secrets"
)

const fakeAuthDBProto = `
groups {
	name: "auth-service-access"
	members: "user:cjacomet@google.com"
	members: "user:vadimsh@google.com"
}

# prpc CLI tool client ID.
oauth_additional_client_ids: "446450136466-2hr92jrq8e6i4tnsa56b52vacp7t3936.apps.googleusercontent.com"
`

// Main launches a server with some default modules and configuration installed.
func Main(modules []module.Module, cb func(srv *server.Server) error) {
	fakeAuthDB, err := authdb.SnapshotDBFromTextProto(strings.NewReader(fakeAuthDBProto))
	if err != nil {
		panic(fmt.Errorf("bad AuthDB text proto: %s", err))
	}

	opts := &server.Options{
		// Options for getting OAuth2 tokens when running the server locally.
		ClientAuth: chromeinfra.SetDefaultAuthOptions(auth.Options{
			Scopes: []string{
				"https://www.googleapis.com/auth/cloud-platform",
				"https://www.googleapis.com/auth/userinfo.email",
			},
		}),
		// Use a fake hardcoded AuthDB for now until we can read the datastore.
		AuthDBProvider: func(context.Context) (authdb.DB, error) {
			return fakeAuthDB, nil
		},
	}

	modules = append([]module.Module{
		gaeemulation.NewModuleFromFlags(), // for accessing Datastore
		secrets.NewModuleFromFlags(),      // for accessing Cloud Secret Manager
	}, modules...)

	server.Main(opts, modules, cb)
}
