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
	var notifiers []*pb.Notifier
	for _, notifier := range cfg.Notifiers {
		for _, notification := range notifier.Notifications {
			for _, builder := range notifier.Builders {
				notifiers = append(notifiers, &pb.Notifier{
					Notifications: []*pb.Notification{notification},
					Builders:      []*pb.Builder{builder},
				})
			}
		}
	}
	sort.SliceStable(notifiers, func(i, j int) bool {
		switch {
		case notifiers[i].Builders[0].Bucket < notifiers[j].Builders[0].Bucket:
			return true
		case notifiers[i].Builders[0].Bucket > notifiers[j].Builders[0].Bucket:
			return false
		default:
			return notifiers[i].Builders[0].Name < notifiers[j].Builders[0].Name
		}
	})
	cfg.Notifiers = notifiers
	return nil
}
