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

package cas

import (
	api "go.chromium.org/luci/cipd/api/cipd/v1"
)

// Internal returns non-ACLed implementation of cas.StorageService.
//
// It can be used internally by the backend. Assumes ACL checks are already
// done.
func Internal() api.StorageServer {
	return &storageImpl{}
}

// storageImpl implements api.StorageServer.
//
// Doesn't do any ACL checks.
type storageImpl struct{}
