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

type mockClientFactory struct {
	gitilesMock gitilespb.GitilesClient
	gerritMock  gerritpb.GerritClient
}

func (m *mockClientFactory) Gitiles(ctx context.Context, host string, project string) (gitilespb.GitilesClient, error) {
	return m.gitilesMock, nil
}

func (m *mockClientFactory) Gerrit(ctx context.Context, host string, project string) (gerritpb.GerritClient, error) {
	return m.gerritMock, nil
}
