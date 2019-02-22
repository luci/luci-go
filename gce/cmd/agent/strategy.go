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

package main

import (
	"context"
)

// PlatformStrategy is a platform-specific strategy.
type PlatformStrategy interface {
	// autostart configures the given Swarming bot code to be executed on startup
	// for the given user, then starts the Swarming bot process.
	autostart(context.Context, string, string) error
	// chown modifies the given path to be owned by the given user.
	chown(context.Context, string, string) error
}
