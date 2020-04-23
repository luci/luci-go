// Copyright 2020 The LUCI Authors.
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

package job

import (
	"context"

	swarming "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/errors"
)

// KitchenSupport is the object for an interface to support 'LegacyKitchen' job
// definitions.
//
// See 'infra/tools/led2' for the only real implementation of this.
//
// This is abstracted out because 'kitchen' and its libraries live in infra, not
// in luci-go. Once kitchen is deleted, this will go away as well.
type KitchenSupport interface {
	FromSwarming(ctx context.Context, in *swarming.SwarmingRpcsNewTaskRequest, out *Buildbucket) error
	GenerateCommand(ctx context.Context, bb *Buildbucket) ([]string, error)
}

// NoKitchenSupport returns a null implementation of KitchenSupport which always
// returns errors if it ends up processing a kitchen job.
func NoKitchenSupport() KitchenSupport {
	return nullKitchenSupport{}
}

type nullKitchenSupport struct{}

func (nullKitchenSupport) FromSwarming(context.Context, *swarming.SwarmingRpcsNewTaskRequest, *Buildbucket) error {
	return errors.New("kitchen job Definitions not supported by this binary")
}

func (nullKitchenSupport) GenerateCommand(context.Context, *Buildbucket) ([]string, error) {
	return nil, errors.New("kitchen job Definitions not supported by this binary")
}
