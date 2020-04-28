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
	// CanaryPreference was used to indicate if the build may use canary infrastructure.
	// Since replaced by the proto's canary field.
	CanaryPreference int32 `gae:"canary_preference,noindex"`
	// Infra was used to store the proto's infra field value in order to reduce
	// the size of the proto being unmarshalled in projection queries.
	// Since moved to its own entity (see details.go) and backfilled for all builds.
	Infra []byte `gae:"infra_bytes,noindex"`
	// InputProperties was used to store the proto's input.properties field value
	// in order to reduce the size of the proto being unmarshalled in projection queries.
	// Since moved to its own entity (see details.go) and backfilled for all builds.
	InputProperties []byte `gae:"input_properties_bytes,noindex"`
	// SwarmingTaskKey was used before Swarming task creation was made idempotent
	// in order to ensure only one task could call UpdateBuild.
	SwarmingTaskKey string `gae:"swarming_task_key,noindex"`
	// PubSubCallback is normally a struct (see build.go), which translates into datastore
	// fields pubsub_callback.auth_token, pubsub_callback.topic, pubsub_callback.user_data
	// with no actual field called pubsub_callback. However, nil values in the datastore
	// may exist for pubsub_callback (should instead be represented by having all three
	// pubsub_callback.* fields nil, but isn't). Capture such nil values here.
	// TODO(crbug/1042991): Support this case properly in gae datastore package.
	PubSubCallback []byte `gae:"pubsub_callback,noindex"`
}
