// Copyright 2022 The LUCI Authors.
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

package listener

import (
	"context"

	"cloud.google.com/go/pubsub"

	listenerpb "go.chromium.org/luci/cv/settings/listener"
)

// gerritProcessor implements processor interface for Gerrit subscription.
type gerritProcessor struct {
	sch  scheduler
	host string
}

// process processes a given Gerrit pubsub message and schedules UpdateCLTask(s)
// for all the LUCI projects watching the Gerrit repo.
func (p *gerritProcessor) process(ctx context.Context, m *pubsub.Message) {
	// TODO: implement me
	m.Ack()
}

func newGerritSubscriber(c *pubsub.Client, sch scheduler, settings *listenerpb.Settings_GerritSubscription) *subscriber {
	subID := settings.GetSubscriptionId()
	if subID == "" {
		subID = settings.GetHost()
	}
	sber := &subscriber{
		sub:  c.Subscription(subID),
		proc: &gerritProcessor{sch: sch, host: settings.GetHost()},
	}
	sber.sub.ReceiveSettings.NumGoroutines = defaultNumGoroutines
	sber.sub.ReceiveSettings.MaxOutstandingMessages = defaultMaxOutstandingMessages
	if val := settings.GetReceiveSettings().GetNumGoroutines(); val != 0 {
		sber.sub.ReceiveSettings.NumGoroutines = int(val)
	}
	if val := settings.GetReceiveSettings().GetMaxOutstandingMessages(); val != 0 {
		sber.sub.ReceiveSettings.MaxOutstandingMessages = int(val)
	}
	return sber
}

// sameGerritSubscriberSettings returns true if a given GerritSubscriber is
// configured with given settings.
func sameGerritSubscriberSettings(sber *subscriber, settings *listenerpb.Settings_GerritSubscription) bool {
	subID := settings.GetSubscriptionId()
	if subID == "" {
		subID = settings.GetHost()
	}
	return (sber.proc.(*gerritProcessor).host == settings.GetHost() &&
		sber.sub.ID() == subID &&
		sber.sameReceiveSettings(settings.GetReceiveSettings()))
}
