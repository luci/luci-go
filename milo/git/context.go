// Copyright 2019 The LUCI Authors.
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
	"context"

	"go.chromium.org/luci/common/errors"
)

var luciProjectKey = "luci_project_ctx"

// WithProject annotates the context object with the LUCI project that
// a request is handled for.
func WithProject(ctx context.Context, project string) context.Context {
	return context.WithValue(ctx, &luciProjectKey, project)
}

// ProjectFromContext is the opposite of WithProject, is extracts the
// LUCI project which the current call stack is handling a request for.
func ProjectFromContext(ctx context.Context) (string, error) {
	project, ok := ctx.Value(&luciProjectKey).(string)
	if !ok {
		return "", errors.Reason("LUCI project not available in context").Err()
	}
	if project == "" {
		return "", errors.Reason("LUCI project is empty string").Err()
	}
	return project, nil
}
