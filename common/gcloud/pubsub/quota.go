// Copyright 2015 The LUCI Authors.
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

package pubsub

import (
	"time"

	"cloud.google.com/go/pubsub"
)

// Cloud PubSub quota is documented here:
// https://cloud.google.com/pubsub/quotas
const (
	// MaxPublishRequestBytes is the maximum size of a single publish request in
	// bytes, as determined by the PubSub service.
	//
	// See: https://cloud.google.com/pubsub/publisher
	MaxPublishRequestBytes = 1e7

	// MaxPublishRequestCount is the maximum PubSub batch size.
	MaxPublishRequestCount = pubsub.MaxPublishRequestCount

	// MaxProjectMessagesPerSecond is the maximum number of requests per second,
	// across the entire project.
	MaxProjectMessagesPerSecond = 10000

	// MaxACKDeadline is the maximum acknowledgement deadline that can be applied
	// to a leased subscription Message.
	MaxACKDeadline = 600 * time.Second
)
