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
	"regexp"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

// ShouldNotify is the predicate function for whether a trigger's conditions have been met.
func (n *Notification) ShouldNotify(oldStatus buildbucketpb.Status, newBuild *buildbucketpb.Build) bool {
	newStatus := newBuild.Status

	switch {

	case newStatus == buildbucketpb.Status_STATUS_UNSPECIFIED:
		panic("new status must always be valid")
	case contains(newStatus, n.OnOccurrence):
	case oldStatus != buildbucketpb.Status_STATUS_UNSPECIFIED && newStatus != oldStatus && contains(newStatus, n.OnNewStatus):

	// deprecated functionality
	case n.OnSuccess && newStatus == buildbucketpb.Status_SUCCESS:
	case n.OnFailure && newStatus == buildbucketpb.Status_FAILURE:
	case n.OnChange && oldStatus != buildbucketpb.Status_STATUS_UNSPECIFIED && newStatus != oldStatus:
	case n.OnNewFailure && newStatus == buildbucketpb.Status_FAILURE && oldStatus != buildbucketpb.Status_FAILURE:

	default:
		return false
	}

	var includeRegex *regexp.Regexp = nil
	if n.FailedStepRegexp != "" {
		// We should never get an invalid regex here, as our validation should catch this.
		// If we do, includeRegex will be nil and hence ignored below.
		includeRegex, _ = regexp.Compile(n.FailedStepRegexp)
	}

	var excludeRegex *regexp.Regexp = nil
	if n.FailedStepRegexpExclude != "" {
		// Ditto.
		excludeRegex, _ = regexp.Compile(n.FailedStepRegexpExclude)
	}

	// Don't scan steps if we have no regexes.
	if includeRegex == nil && excludeRegex == nil {
		return true
	}

	for _, step := range newBuild.Steps {
		if step.Status == buildbucketpb.Status_FAILURE {
			if (includeRegex == nil || includeRegex.MatchString(step.Name)) &&
				(excludeRegex == nil || !excludeRegex.MatchString(step.Name)) {
				return true
			}
		}
	}
	return false
}

// Filter filters out Notification objects from Notifications by checking if we ShouldNotify
// based on two build statuses.
func (n *Notifications) Filter(oldStatus buildbucketpb.Status, newBuild *buildbucketpb.Build) Notifications {
	notifications := n.GetNotifications()
	filtered := make([]*Notification, 0, len(notifications))
	for _, notification := range notifications {
		if notification.ShouldNotify(oldStatus, newBuild) {
			filtered = append(filtered, notification)
		}
	}
	return Notifications{Notifications: filtered}
}

// contains checks whether or not a build status is in a list of build statuses.
func contains(status buildbucketpb.Status, statusList []buildbucketpb.Status) bool {
	for _, s := range statusList {
		if status == s {
			return true
		}
	}
	return false
}
