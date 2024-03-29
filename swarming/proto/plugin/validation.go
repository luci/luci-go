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

package pluginpb

import (
	"errors"
	"fmt"
)

// Validate returns an error if r is invalid.
func (r *NotifyTasksRequest) Validate() error {
	if r.SchedulerId == "" {
		return errors.New("SchedulerId is required")
	}
	for i, n := range r.Notifications {
		if n.Time == nil {
			return fmt.Errorf("notification time is required (item %d)", i)
		}
		if n.Task == nil {
			return fmt.Errorf("notification task is required (item %d)", i)
		}
		if n.Task.EnqueuedTime == nil {
			return fmt.Errorf("notification task enqueued time is required (item %d)", i)
		}
	}
	return nil
}

// Validate returns an error if r is invalid.
func (r *AssignTasksRequest) Validate() error {
	if r.SchedulerId == "" {
		return errors.New("SchedulerId is required")
	}
	if r.Time == nil {
		return errors.New("Time is required")
	}
	return nil
}

// Validate returns an error if r is invalid.
func (r *GetCancellationsRequest) Validate() error {
	if r.SchedulerId == "" {
		return errors.New("SchedulerId is required")
	}
	return nil
}
