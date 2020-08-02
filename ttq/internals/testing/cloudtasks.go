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

package testing

import (
	"context"
	"sync"

	"github.com/googleapis/gax-go/v2"

	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
)

// FakeCloudTasks records CreateTasks calls and allows to fake errors.
type FakeCloudTasks struct {
	mu    sync.Mutex
	errs  []error
	calls []*taskspb.CreateTaskRequest
}

// FakeCreateTaskErrors presets 1 or more errors to be returned
// in the following CreateTask calls.
func (f *FakeCloudTasks) FakeCreateTaskErrors(errs ...error) {
	f.mu.Lock()
	f.errs = errs
	f.mu.Unlock()
}

// PopCreateTaskReqs returns CreateTaskRequest from prior CreateTask calls.
func (f *FakeCloudTasks) PopCreateTaskReqs() []*taskspb.CreateTaskRequest {
	f.mu.Lock()
	ret := f.calls
	f.calls = nil
	f.mu.Unlock()
	return ret
}

// CreateTask fakes task creation.
func (f *FakeCloudTasks) CreateTask(_ context.Context, req *taskspb.CreateTaskRequest, _ ...gax.CallOption) (*taskspb.Task, error) {
	// NOTE: actual implementation returns the created Task, but the TTQ library
	// doesn't care.
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req)
	if len(f.errs) > 0 {
		var err error
		f.errs, err = f.errs[1:], f.errs[0]
		return nil, err
	}
	// Actual CreateTasks returns created Task, notably with auto-assigned name
	// if req.Task wasn't named, but TTQ doesn't currently care.
	return nil, nil
}
