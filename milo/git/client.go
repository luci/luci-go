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

package git

import (
	"net/http"

	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/milo/api/config"
	"go.chromium.org/luci/server/auth"
	"golang.org/x/net/context"
)

// ClientFactory creates clients for Gerrit and Gitiles.
type ClientFactory interface {
	Gitiles(ctx context.Context, host, project string) (gitilespb.GitilesClient, error)
	Gerrit(ctx context.Context, host, project string) (gerritpb.GerritClient, error)
}

var factoryKey = "gitiles client factory key"

// UseFactory installs f into c.
func UseFactory(c context.Context, f ClientFactory) context.Context {
	return context.WithValue(c, &factoryKey, f)
}

func NewProdClientFactory(settings *config.Settings) (ClientFactory, error) {
	acls, err := ACLsFromConfig(settings.SourceAcls)
	if err != nil {
		return nil, errors.Annotate(err, "source_acls config invalid").Err()
	}
	return &prodClientFactory{*acls}, nil
}

// private implementation

// prodClientFactory implements ClientFactory.
type prodClientFactory struct {
	acls ACLs
}

func (f *prodClientFactory) Gitiles(c context.Context, host, project string) (gitilespb.GitilesClient, error) {
	t, auth, err := f.transport(c, host, project)
	if err != nil {
		return nil, err
	}
	return gitiles.NewRESTClient(&http.Client{Transport: t}, host, auth)
}
func (f *prodClientFactory) Gerrit(c context.Context, host, project string) (gerritpb.GerritClient, error) {
	t, auth, err := f.transport(c, host, project)
	if err != nil {
		return nil, err
	}
	return gerrit.NewRESTClient(&http.Client{Transport: t}, host, auth)
}

func (f *prodClientFactory) transport(c context.Context, host, project string) (transport http.RoundTripper, authenticated bool, err error) {
	// TODO(tandrii): if current context has OAuth 2.0 bearer token, use it.
	switch authenticated, err = f.acls.ReadGranted(c, host, project); {
	case err != nil:
		return
	case authenticated:
		transport, err = auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(gitiles.OAuthScope))
	default:
		// TODO(tandrii): fail instead once existing projects work as intended.
		transport, err = auth.GetRPCTransport(c, auth.NoAuth)
	}
	return
}

// gitilesClient creates a new Gitiles client using the ClientFactory installed in c.
// See also UseFactory.
func gitilesClient(c context.Context, host, project string) (gitilespb.GitilesClient, error) {
	f, ok := c.Value(&factoryKey).(ClientFactory)
	if !ok {
		return nil, errors.New("git client factory is not installed in context")
	}
	return f.Gitiles(c, host, project)
}

// gerritClient creates a new Gerrit client using the ClientFactory installed in c.
// See also UseFactory.
func gerritClient(c context.Context, host, project string) (gerritpb.GerritClient, error) {
	f, ok := c.Value(&factoryKey).(ClientFactory)
	if !ok {
		return nil, errors.New("git client factory is not installed in context")
	}
	return f.Gerrit(c, host, project)
}
