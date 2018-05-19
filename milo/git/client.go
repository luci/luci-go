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

// Factory creates clients for Gerrit and Gitiles and provides for ACLs checks.
type Factory interface {
	Gitiles(ctx context.Context, host, project string) (gitilespb.GitilesClient, error)
	Gerrit(ctx context.Context, host, project string) (gerritpb.GerritClient, error)
	IsAllowed(ctx context.Context, host, project string) (bool, error)
}

// UseProdFactory returns context with production Gerrit and Gitiles client
// factory installed.
func UseProdFactory(c context.Context, settings *config.Settings) (context.Context, error) {
	acls, err := ACLsFromConfig(settings.SourceAcls)
	if err != nil {
		return nil, errors.Annotate(err, "source_acls config invalid").Err()
	}
	return UseFactory(c, &prodFactory{acls}), nil
}

// UseFactory returns with context with mock Gerrit and Gitiles client
// factory installed.
func UseFactory(c context.Context, f Factory) context.Context {
	return context.WithValue(c, &factoryKey, f)
}

// private implementation

var factoryKey = "client factory key"

// projectUnknownAssumeAllowed indicates that caller doesn't know project yet,
// but wants a Gerrit or Gitiles client as if current user has access to a
// project.
const projectUnknownAssumeAllowed = "\x00[[UnknownAssumeAllowed]]"

// prodFactory implements Factory.
type prodFactory struct {
	acls *ACLs
}

func (f *prodFactory) Gitiles(c context.Context, host, project string) (gitilespb.GitilesClient, error) {
	t, auth, err := f.transport(c, host, project)
	if err != nil {
		return nil, err
	}
	return gitiles.NewRESTClient(&http.Client{Transport: t}, host, auth)
}
func (f *prodFactory) Gerrit(c context.Context, host, project string) (gerritpb.GerritClient, error) {
	t, auth, err := f.transport(c, host, project)
	if err != nil {
		return nil, err
	}
	return gerrit.NewRESTClient(&http.Client{Transport: t}, host, auth)
}

func (f *prodFactory) IsAllowed(c context.Context, host, project string) (bool, error) {
	return f.acls.IsAllowed(c, host, project)
}

func (f *prodFactory) transport(c context.Context, host, project string) (transport http.RoundTripper, authenticated bool, err error) {
	if project == projectUnknownAssumeAllowed {
		authenticated = true
		transport, err = auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(gitiles.OAuthScope))
		return
	}
	// TODO(tandrii): if current context has OAuth 2.0 bearer token, use it.
	// TODO(tandrii): instead of auth.Self, use service accounts configured per
	//   LUCI project ( != Git/Gerrit project ).
	switch authenticated, err = f.acls.IsAllowed(c, host, project); {
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

var errFactoryNotInstalled = errors.New("git client factory is not installed in context")

func gitilesClient(c context.Context, host, project string) (gitilespb.GitilesClient, error) {
	f, ok := c.Value(&factoryKey).(Factory)
	if !ok {
		return nil, errFactoryNotInstalled
	}
	return f.Gitiles(c, host, project)
}

func gerritClient(c context.Context, host, project string) (gerritpb.GerritClient, error) {
	f, ok := c.Value(&factoryKey).(Factory)
	if !ok {
		return nil, errFactoryNotInstalled
	}
	return f.Gerrit(c, host, project)
}

func isAllowed(c context.Context, host, project string) (bool, error) {
	f, ok := c.Value(&factoryKey).(Factory)
	if !ok {
		return false, errFactoryNotInstalled
	}
	return f.IsAllowed(c, host, project)
}
