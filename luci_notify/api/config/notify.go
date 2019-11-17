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

	// deprecated functionality
	case n.OnSuccess && newStatus == buildbucketpb.Status_SUCCESS:
	case n.OnFailure && newStatus == buildbucketpb.Status_FAILURE:
	case n.OnChange && oldStatus != buildbucketpb.Status_STATUS_UNSPECIFIED && newStatus != oldStatus:
	case n.OnNewFailure && newStatus == buildbucketpb.Status_FAILURE && oldStatus != buildbucketpb.Status_FAILURE:

	case oldStatus == buildbucketpb.Status_STATUS_UNSPECIFIED || newStatus == oldStatus:
		return n.matches(newStatus, false)
	default:
		return n.matches(newStatus, true)
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

// matches checks whether or not a build status matches a Notification's NotificationStatus.
func (n *Notification) matches(status buildbucketpb.Status, includeChange bool) bool {
	switch status {
	case buildbucketpb.Status_STATUS_UNSPECIFIED:
		return (n.OnOccurrence != nil && n.OnOccurrence.StatusStatusUnspecified) || (includeChange && n.OnNewStatus != nil && n.OnNewStatus.StatusStatusUnspecified)
	case buildbucketpb.Status_SCHEDULED:
		return (n.OnOccurrence != nil && n.OnOccurrence.StatusScheduled) || (includeChange && n.OnNewStatus != nil && n.OnNewStatus.StatusScheduled)
	case buildbucketpb.Status_STARTED:
		return (n.OnOccurrence != nil && n.OnOccurrence.StatusStarted) || (includeChange && n.OnNewStatus != nil && n.OnNewStatus.StatusStarted)
	case buildbucketpb.Status_ENDED_MASK:
		return (n.OnOccurrence != nil && n.OnOccurrence.StatusEndedMask) || (includeChange && n.OnNewStatus != nil && n.OnNewStatus.StatusEndedMask)
	case buildbucketpb.Status_SUCCESS:
		return (n.OnOccurrence != nil && n.OnOccurrence.StatusSuccess) || (includeChange && n.OnNewStatus != nil && n.OnNewStatus.StatusSuccess)
	case buildbucketpb.Status_FAILURE:
		return (n.OnOccurrence != nil && n.OnOccurrence.StatusFailure) || (includeChange && n.OnNewStatus != nil && n.OnNewStatus.StatusFailure)
	case buildbucketpb.Status_INFRA_FAILURE:
		return (n.OnOccurrence != nil && n.OnOccurrence.StatusInfraFailure) || (includeChange && n.OnNewStatus != nil && n.OnNewStatus.StatusInfraFailure)
	case buildbucketpb.Status_CANCELED:
		return (n.OnOccurrence != nil && n.OnOccurrence.StatusCanceled) || (includeChange && n.OnNewStatus != nil && n.OnNewStatus.StatusCanceled)
	default:
		return false
	}
}
