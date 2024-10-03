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
	"fmt"
	"sync"
	"time"

	"cloud.google.com/go/pubsub"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/configs/srvcfg"
	listenerpb "go.chromium.org/luci/cv/settings/listener"
)

const (
	// reloadInterval defines how often Listener evaluates the latest copy of
	// the subscription settings and adjusts subscribers as necessary.
	reloadInterval = time.Minute
)

// This interface encapsulate the communication with changelist.Updater.
type scheduler interface {
	Schedule(context.Context, *changelist.UpdateCLTask) error
}

// Listener fetches and process messages from the subscriptions configured
// in the settings.
type Listener struct {
	mu        sync.Mutex
	sbers     map[string]*subscriber
	sch       scheduler
	psClient  *pubsub.Client
	prjFinder *projectFinder
}

// NewListener constructs a Listener.
func NewListener(psClient *pubsub.Client, sch scheduler) *Listener {
	return &Listener{
		sbers:     make(map[string]*subscriber),
		sch:       sch,
		psClient:  psClient,
		prjFinder: &projectFinder{},
	}
}

// Run continuously evaluates the listener settings and manages workers
// for each of the subscriptions configured.
func (l *Listener) Run(ctx context.Context) {
	var prevHash string

	for {
		meta := &config.Meta{}
		lcfg, err := srvcfg.GetListenerConfig(ctx, meta)
		switch {
		case err != nil:
			logging.Errorf(ctx, "GetListenerConfig: %s", err)
		case meta.ContentHash != prevHash:
			// new config?
			logging.Infof(ctx, "Listener.Run: new settings.cfg found: %s", meta.ContentHash)
			if err := l.reload(ctx, lcfg); err != nil {
				logging.Errorf(ctx, "Listener.reload: %s", err.Error())
			}
		}

		select {
		case <-ctx.Done():
			logging.Infof(ctx, "Listener.Run: the context is done; exiting")
			return
		case <-clock.After(ctx, reloadInterval):
		}
	}
}

func (l *Listener) reload(ctx context.Context, s *listenerpb.Settings) error {
	if err := l.prjFinder.reload(s); err != nil {
		return errors.Annotate(err, "projectFinder.reload").Err()
	}
	if err := l.reloadSubscribers(ctx, s.GetGerritSubscriptions()); err != nil {
		return errors.Annotate(err, "reloadSubscribers").Err()
	}
	return nil
}

// reloadSubscribers reloads the subscribers as configured in the settings.
//
// It will
// - start a subscriber for new subscription settings.
// - stop the subscriber for removed subscription settings.
// - restart the subscriber if it is found dead.
func (l *Listener) reloadSubscribers(ctx context.Context, settings []*listenerpb.Settings_GerritSubscription) error {
	var wg sync.WaitGroup
	activeHosts := stringset.New(len(settings))
	startErrs := errors.NewLazyMultiError(len(settings))
	l.mu.Lock()
	defer l.mu.Unlock()

	for i, setting := range settings {
		i, setting := i, setting
		host := setting.GetHost()
		if !activeHosts.Add(host) {
			return fmt.Errorf("duplicate host %q; there must be a bug in the cfg validation", host)
		}

		switch sber, ok := l.sbers[host]; {
		case ctx.Err() != nil:
			return ctx.Err()

		case !ok:
			sber = newGerritSubscriber(l.psClient, l.sch, l.prjFinder, setting)
			logging.Infof(ctx, "listener.reload: new host %q found; starting a subscriber", host)
			if err := sber.start(ctx); err != nil {
				startErrs.Assign(i, err)
			}
			l.sbers[host] = sber

		// If the setting changed, stop the existing subscriber and
		// start a new one.
		case !sameGerritSubscriberSettings(ctx, sber, setting):
			logging.Infof(ctx, "listener.reload: subscriber setting changed for host %q", host)
			wg.Add(1)
			newSber := newGerritSubscriber(l.psClient, l.sch, l.prjFinder, setting)
			l.sbers[host] = newSber

			go func() {
				defer wg.Done()
				sber.stop(ctx)
				if err := newSber.start(ctx); err != nil {
					startErrs.Assign(i, err)
				}
			}()

		// did the subscriber stop or fail to start?
		case sber.isStopped():
			logging.Warningf(ctx, "listener.reload: subscriber for host %q was found dead; restarting", host)
			if err := sber.start(ctx); err != nil {
				startErrs.Assign(i, err)
			}
		}
	}

	// stop the Gerrit subscribers for the removed Gerrit hosts.
	for host, sber := range l.sbers {
		if !activeHosts.Has(host) {
			logging.Infof(ctx, "listener.reload: host %q was removed from the settings", host)
			delete(l.sbers, host)

			sber := sber
			wg.Add(1)
			go func() {
				defer wg.Done()
				sber.stop(ctx)
			}()
		}
	}
	wg.Wait()
	return startErrs.Get()
}

// getSubscriber returns the subscriber for a given host.
//
// Returns nil if there isn't any.
func (l *Listener) getSubscriber(host string) *subscriber {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sbers[host]
}
