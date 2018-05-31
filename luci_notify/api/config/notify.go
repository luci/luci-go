// Copyright 2018 The LUCI Authors.
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

package config

import (
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

// ShouldNotify is the predicate function for whether a trigger's conditions have been met.
func (n *Notification) ShouldNotify(oldStatus, newStatus buildbucketpb.Status) bool {
	switch {
	case newStatus == buildbucketpb.Status_STATUS_UNSPECIFIED:
		panic("new status must always be valid")
	case n.OnSuccess && newStatus == buildbucketpb.Status_SUCCESS:
	case n.OnFailure && newStatus == buildbucketpb.Status_FAILURE:
	case n.OnChange && oldStatus != buildbucketpb.Status_STATUS_UNSPECIFIED && newStatus != oldStatus:
	default:
		return false
	}
	return true
}

// Filter filters out Notification objects from Notifications by checking if we ShouldNotify
// based on two build statuses.
func (n *Notifications) Filter(oldStatus, newStatus buildbucketpb.Status) Notifications {
	notifications := n.GetNotifications()
	filtered := make([]*Notification, 0, len(notifications))
	for _, notification := range notifications {
		if notification.ShouldNotify(oldStatus, newStatus) {
			filtered = append(filtered, notification)
		}
	}
	return Notifications{Notifications: filtered}
}
