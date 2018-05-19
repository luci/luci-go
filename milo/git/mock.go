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
	"golang.org/x/net/context"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
)

// MockFactory implements Factory.
type MockFactory struct {
	GerritMock    gerritpb.GerritClient
	GitilesMock   gitilespb.GitilesClient
	IsAllowedMock func(ctx context.Context, host string, project string) (bool, error)
}

func (m *MockFactory) Gerrit(ctx context.Context, host string, project string) (gerritpb.GerritClient, error) {
	return m.GerritMock, nil
}

func (m *MockFactory) Gitiles(ctx context.Context, host string, project string) (gitilespb.GitilesClient, error) {
	return m.GitilesMock, nil
}

func (m *MockFactory) IsAllowed(ctx context.Context, host string, project string) (bool, error) {
	return m.IsAllowedMock(ctx, host, project)
}

// MockAllowedEverything implements IsAllowed by always returning true.
func MockAllowedEverything(ctx context.Context, host string, project string) (bool, error) {
	return true, nil
}
