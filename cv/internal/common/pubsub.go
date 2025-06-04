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

package common

// PubSubMessagePayload is the payload of a Google Cloud Pub/Sub event.
//
// Source: https://cloud.google.com/pubsub/docs/payload-unwrapping#wrapped-message
type PubSubMessagePayload struct {
	Message      PubSubMessage `json:"message"`
	Subscription string        `json:"subscription"`
}

// PubSubMessage is the message body in the PubSubMessagePayload.
type PubSubMessage struct {
	Data        string            `json:"data"` // This is base64-encoded
	MessageID   string            `json:"message_id"`
	PublishTime string            `json:"publish_time"`
	Attributes  map[string]string `json:"attributes"`
}
