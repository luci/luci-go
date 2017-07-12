// Copyright 2015 The LUCI Authors.
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

package authdbimpl

import (
	"golang.org/x/net/context"

	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/server/auth/service"
)

// authService is interface for service.AuthService.
//
// Unit tests inject fake implementation into the testing context.
type authService interface {
	EnsureSubscription(c context.Context, subscription, pushURL string) error
	DeleteSubscription(c context.Context, subscription string) error
	PullPubSub(c context.Context, subscription string) (*service.Notification, error)
	ProcessPubSubPush(c context.Context, body []byte) (*service.Notification, error)
	GetLatestSnapshotRevision(c context.Context) (int64, error)
	GetSnapshot(c context.Context, rev int64) (*service.Snapshot, error)
}

type contextKey int

// setAuthService injects authService implementation into the context.
//
// Used in unit tests.
func setAuthService(c context.Context, s authService) context.Context {
	return context.WithValue(c, contextKey(0), s)
}

// getAuthService returns authService implementation injected into the context
// via setAuthService or *service.AuthService otherwise.
func getAuthService(c context.Context, url string) authService {
	if s, _ := c.Value(contextKey(0)).(authService); s != nil {
		return s
	}
	return &service.AuthService{URL: url}
}

// defaultNS returns GAE context configured to use default namespace.
//
// All publicly callable functions must use it to switch to default namespace.
// All internal functions expect the context to be in the default namespace.
//
// Idempotent.
func defaultNS(c context.Context) context.Context {
	return info.MustNamespace(c, "")
}
