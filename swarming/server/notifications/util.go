// Copyright 2024 The LUCI Authors.
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

// Package notifications contains the logic about send Swarming notifications.
package notifications

import (
	"regexp"

	"go.chromium.org/luci/common/errors"
)

var (
	// topicNameRE is the full topic name regex.
	topicNameRE = regexp.MustCompile(`^projects/(.*)/topics/(.*)$`)
)

// parsePubSubTopicName parses the full topic name.
func parsePubSubTopicName(topic string) (string, string, error) {
	matches := topicNameRE.FindAllStringSubmatch(topic, -1)
	if matches == nil || len(matches[0]) != 3 {
		return "", "", errors.Fmt("topic %q does not match %q", topic, topicNameRE)
	}

	return matches[0][1], matches[0][2], nil
}
