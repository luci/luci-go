// Copyright 2022 The LUCI Authors.
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

//go:build !copybara
// +build !copybara

package cipd

import (
	"context"

	"go.chromium.org/luci/cipd/client/cipd/plugin"
	"go.chromium.org/luci/cipd/client/cipd/plugin/host"
)

// Note: this file is excluded from copybara to avoid pulling a dependency on
// go.chromium.org/luci/cipd/client/cipd/plugin/host which causes issues
// downstream related to presence of a gRPC server there.

func init() {
	initPluginHost = func(ctx context.Context) plugin.Host {
		return &host.Host{PluginsContext: ctx}
	}
}
