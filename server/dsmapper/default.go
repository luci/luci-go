// Copyright 2021 The LUCI Authors.
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

package dsmapper

import (
	"context"
)

// Default is a controller initialized by the server module.
var Default = Controller{}

// RegisterFactory adds the given mapper factory to the internal registry.
//
// See Controller.RegisterFactory for details.
func RegisterFactory(id ID, m Factory) {
	Default.RegisterFactory(id, m)
}

// LaunchJob launches a new mapping job, returning its ID.
//
// See Controller.LaunchJob for details.
func LaunchJob(ctx context.Context, j *JobConfig) (JobID, error) {
	return Default.LaunchJob(ctx, j)
}
