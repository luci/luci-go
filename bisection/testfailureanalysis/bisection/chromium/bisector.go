// Copyright 2023 The LUCI Authors.
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

// Package chromium performs bisection for test failures for Chromium project.
package chromium

import (
	"context"

	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/common/logging"
)

// Run runs bisection for the given analysisID.
func Run(ctx context.Context, tfa *model.TestFailureAnalysis) error {
	logging.Infof(ctx, "Run chromium bisection")
	return nil
}
