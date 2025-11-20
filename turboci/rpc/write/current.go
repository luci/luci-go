// Copyright 2025 The LUCI Authors.
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

package write

import (
	"fmt"

	"go.chromium.org/luci/common/proto/delta"

	"go.chromium.org/luci/turboci/rpc/write/current"
)

// Current returns an Diff which sets the CurrentStage field for this write.
//
// Note that Current covers many aspects of the current attempt as well as the
// current stage.
//
// When calling the WriteNodes API, the current stage and attempt are implied by
// the required and validated StageAttemptToken.
func Current(diffs ...*current.Diff) *Diff {
	cs, err := delta.Collect(diffs...)
	if err != nil {
		err = fmt.Errorf("write.Current: %w", err)
	}
	return template.New(builder{
		CurrentStage: cs,
	}, err)
}
