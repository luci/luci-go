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

package model

// UnusedProperties are properties previously set but currently unused.
type UnusedProperties struct {
	// PubSubCallback is normally a struct (see build.go), which translates into datastore
	// fields pubsub_callback.auth_token, pubsub_callback.topic, pubsub_callback.user_data
	// with no actual field called pubsub_callback. However, nil values in the datastore
	// may exist for pubsub_callback (should instead be represented by having all three
	// pubsub_callback.* fields nil, but isn't). Capture such nil values here.
	// TODO(crbug/1042991): Support this case properly in gae datastore package.
	PubSubCallback []byte `gae:"pubsub_callback,noindex"`
}
