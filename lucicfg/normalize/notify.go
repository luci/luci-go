// Copyright 2019 The LUCI Authors.
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

package normalize

import (
	"context"
	"sort"

	pb "go.chromium.org/luci/luci_notify/api/config"
)

// Notify normalizes luci-notify.cfg config.
func Notify(c context.Context, cfg *pb.ProjectConfig) error {
	var notifiers []*pb.Notifier = nil
	for _, notifier := range cfg.Notifiers {
		for _, notification := range notifier.Notifications {
			for _, builder := range notifier.Builders {
				new_notifier := &pb.Notifier{}
				new_notifier.Notifications = []*pb.Notification{notification}
				new_notifier.Builders = []*pb.Builder{builder}
				notifiers = append(notifiers, new_notifier)
			}
		}
	}
	sort.Slice(notifiers, func(i, j int) bool {
		return notifiers[i].Builders[0].Bucket < notifiers[j].Builders[0].Bucket
	})
	sort.SliceStable(notifiers, func(i, j int) bool {
		return notifiers[i].Builders[0].Name < notifiers[j].Builders[0].Name
	})
	cfg.Notifiers = notifiers
	return nil
}
