// Copyright 2024 The LUCI Authors.
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

// Package botapi implements core Bot API handlers.
//
// Handlers related to RBE are in the "rbe" package for now.
package botapi

import (
	"context"

	"go.chromium.org/luci/common/data/caching/lru"
	minterpb "go.chromium.org/luci/tokenserver/api/minter/v1"

	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/botinfo"
	"go.chromium.org/luci/swarming/server/botsrv"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/hmactoken"
	"go.chromium.org/luci/swarming/server/tasks"
)

// BotAPIServer implements core Bot API handlers.
//
// Handlers are implement in individual Go files. They are all installed into
// the server router in main.go.
type BotAPIServer struct {
	// cfg is the server config.
	cfg *cfg.Provider
	// tasksManager is used to change state of tasks.
	tasksManager tasks.Manager
	// hmacSecret is used to generate new session tokens.
	hmacSecret *hmactoken.Secret
	// project is the Swarming Cloud Project name.
	project string
	// version is the server's version
	version string
	// botCodeCache is the cache of the bot code blobs to avoid hitting datastore.
	botCodeCache *lru.Cache[string, []byte]
	// authorizeBot is botsrv.AuthorizeBot, but it can be mocked in tests.
	authorizeBot func(ctx context.Context, botID string, methods []*configpb.BotAuth) error
	// submitUpdate calls u.Submit, but it can be mocked in tests.
	submitUpdate func(ctx context.Context, u *botinfo.Update) error
	// tokenServerClient produces a Token Server client, can be mocked in tests.
	tokenServerClient func(ctx context.Context, realm string) (minterpb.TokenMinterClient, error)
}

// NewBotAPIServer constructs a new BotAPIServer.
func NewBotAPIServer(cfg *cfg.Provider, tasksManager tasks.Manager, secret *hmactoken.Secret, project, version string) *BotAPIServer {
	return &BotAPIServer{
		cfg:          cfg,
		tasksManager: tasksManager,
		hmacSecret:   secret,
		project:      project,
		version:      version,
		botCodeCache: lru.New[string, []byte](2), // two versions: canary + stable
		authorizeBot: botsrv.AuthorizeBot,
		submitUpdate: func(ctx context.Context, u *botinfo.Update) error {
			if u.TasksManager != tasksManager {
				panic("u.TasksManager isn't populated")
			}
			_, err := u.Submit(ctx)
			return err
		},
		tokenServerClient: tokenServerClient, // see tokens.go
	}
}

// UnimplementedRequest is used as a placeholder in unimplemented handlers.
type UnimplementedRequest struct{}

func (r *UnimplementedRequest) ExtractSession() []byte   { return nil }
func (r *UnimplementedRequest) ExtractDebugRequest() any { return nil }
