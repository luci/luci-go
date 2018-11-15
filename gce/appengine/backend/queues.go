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

package backend

import (
	"context"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/gce/api/tasks/v1"
)

// expQueue is the name of the expansion task queue.
const expQueue = "expand-config"

// expand creates task queue tasks to process each VM in the given VMs block.
func expand(c context.Context, payload proto.Message) error {
	_, ok := payload.(*tasks.Expansion)
	if !ok {
		return errors.Reason("unexpected payload %q", payload).Err()
	}
	// TODO(smut): Expand the VMs block by creating entities for each configured VM.
	return nil
}
