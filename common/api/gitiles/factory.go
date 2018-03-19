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

package gitiles

import (
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/gitiles"
)

// ClientFactory creates a Gitiles client.
type ClientFactory func(ctx context.Context, host string) (gitiles.GitilesClient, error)

var factoryKey = "gitiles client factory key"

// UseFactory installs f into c.
func UseFactory(c context.Context, f ClientFactory) context.Context {
	return context.WithValue(c, &factoryKey, f)
}

// NewClient creates a new Gitiles client using the ClientFactory installed in c.
// See also UseFactory.
func NewClient(c context.Context, host string) (gitiles.GitilesClient, error) {
	f, ok := c.Value(&factoryKey).(ClientFactory)
	if !ok {
		return nil, errors.New("gitiles client factory is not installed in context")
	}
	return f(c, host)
}
