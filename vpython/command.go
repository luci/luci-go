// Copyright 2017 The LUCI Authors.
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

package vpython

import (
	"strings"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"

	"golang.org/x/net/context"
)

const (
	// ApplicationCommandENV is an enviornment varible which, if present,
	// instructs vpython jump to a command routine instead of its default
	// interactive behavior.
	ApplicationCommandENV = "_VPYTHON_APPLICATION_COMMAND"
)

// RunCommand runs the named application command, returning an application
// return code.
func RunCommand(c context.Context, v string, env environ.Env) int {
	parts := strings.SplitN(v, ":", 2)
	name, val := parts[0], ""
	if len(parts) == 2 {
		val = parts[1]
	}

	if rc, ok := runSystemCommand(c, name, val, env); ok {
		return rc
	}

	logging.Errorf(c, "Unknown application command: %q", name)
	return 1
}
