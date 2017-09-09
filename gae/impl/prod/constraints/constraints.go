// Copyright 2016 The LUCI Authors.
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

// Package constraints contains production datastore constraints.
package constraints

import (
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/taskqueue"
)

// DS returns a datastore.Constraints object for the production datastore.
func DS() datastore.Constraints {
	return datastore.Constraints{
		MaxGetSize:    1000,
		MaxPutSize:    500,
		MaxDeleteSize: 500,
	}
}

// TQ returns a taskqueue.Constraints object for the production task queue
// service.
func TQ() taskqueue.Constraints {
	return taskqueue.Constraints{
		MaxAddSize:    100,
		MaxDeleteSize: 1000,
	}
}
