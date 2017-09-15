// Copyright 2017 The LUCI Authors.
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
	"encoding/json"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	notifyConfig "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/buildbucket"
)

// NotificationConfig represents a single notification to send, defined by
// a set of recipients for all support communication channels. It also describes
// what triggers to send the notification on.
type NotificationConfig struct {
	// OnSuccess, if set to true, indicates that one should send this
	// notification on each build success.
	OnSuccess bool `json:"on_success"`

	// OnFailure, if set to true, indicates that one should send this
	// notification on each build failure.
	OnFailure bool `json:"on_failure"`

	// OnChange, if set to true, indicates that one should send this
	// notification only when the build result changes.
	OnChange bool `json:"on_change"`

	// EmailRecipients is a list of email recipients to notify.
	EmailRecipients []string `json:"email_recipients"`

	// Blamelist, if true, emails the build's blamelist on the specified triggers.
	Blamelist bool `json:"blamelist"`
}

func newNotificationConfig(cfg *notifyConfig.Notification) NotificationConfig {
	var recipients []string
	var blamelist bool
	if cfg.Email != nil {
		recipients = cfg.Email.Recipients
		blamelist = cfg.Email.Blamelist
	}
	return NotificationConfig{
		OnSuccess:       cfg.OnSuccess,
		OnFailure:       cfg.OnFailure,
		OnChange:        cfg.OnChange,
		EmailRecipients: recipients,
		Blamelist:       blamelist,
	}
}

// Notifier represents a collection of notifications to send for specific
// builders. Each notification then holds the information for who to send
// the notifications to and on what triggers.
type Notifier struct {
	// Parent is a datastore key to this Notifier's parent (which will always
	// be a Project), effectively making it a child of a specific project.
	Parent *datastore.Key `gae:"$parent"`

	// Name is the Notifier's identifier which is unique within a project.
	Name string `gae:"$id"`

	// BuilderIDs are the builders that this Notifier should monitor.
	BuilderIDs []string

	// Categories are the respective categories for each builder.
	// This field must match BuilderIDs in length.
	Categories []string

	// EncodedNotifications contains the notification configurations as
	// JSON when in the datastore.
	EncodedNotifications []byte `gae:",noindex"`

	// Notifications is a list of notification configurations.
	//
	// This field is not stored in the datastore, and is instead encoded
	// as JSON and placed into EncodedNotifications on a datastore Put.
	//
	// EncodedNotifications is similarly decoded into this field on a
	// datastore Get.
	Notifications []NotificationConfig `gae:"-"`
}

// NewNotifier constructs a new Notifier from a parent key and a Notifier
// as defined by the project configuration proto.
func NewNotifier(parent *datastore.Key, cfg *notifyConfig.Notifier) *Notifier {
	var builderIDs []string
	var categories []string
	for _, b := range cfg.Builders {
		if b.Name != "" {
			builderIDs = append(builderIDs, b.Name)
			categories = append(categories, b.Category)
		}
	}
	var notifications []NotificationConfig
	for _, n := range cfg.Notifications {
		c := newNotificationConfig(n)
		notifications = append(notifications, c)
	}
	return &Notifier{
		Parent:        parent,
		Name:          cfg.Name,
		BuilderIDs:    builderIDs,
		Categories:    categories,
		Notifications: notifications,
	}
}

// GetNotifier gets a single, specific Notifier from the datastore.
func GetNotifier(c context.Context, project, name string) (*Notifier, error) {
	n := Notifier{
		Parent: datastore.MakeKey(c, "Project", project),
		Name:   name,
	}
	err := datastore.Get(c, &n)
	return &n, err
}

// LookupNotifiers retrieves all notifiers from the datastore for a particular builder ID.
func LookupNotifiers(c context.Context, build *buildbucket.BuildInfo) []*Notifier {
	var notifiers []*Notifier
	query := datastore.NewQuery("Notifier").Eq("BuilderIDs", build.GetBuilderID())
	if err := datastore.GetAll(c, query, &notifiers); err != nil {
		logging.WithError(err).Warningf(c, "failed to get Notifiers for builder")
	}
	return notifiers
}

// UpdateDatastore stores the Notifier into the datastore.
func (n *Notifier) UpdateDatastore(c context.Context) error {
	if err := datastore.Put(c, n); err != nil {
		return errors.Annotate(err, "saving %s", n.Name).Err()
	}
	return nil
}

// Load loads a Notifier's information from the datastore.
//
// This implements PropertyLoadSaver. Load decodes the EncodedNotifications
// field into Notifications after a successful datastore load.
func (n *Notifier) Load(props datastore.PropertyMap) error {
	if err := datastore.GetPLS(n).Load(props); err != nil {
		return err
	}
	if err := json.Unmarshal(n.EncodedNotifications, &n.Notifications); err != nil {
		return err
	}
	return nil
}

// Save saves a Notifier's information to the datastore.
//
// This implements PropertyLoadSaver. Save encodes the Notifications
// field into EncodedNotifications before a datastore save.
func (n *Notifier) Save(withMeta bool) (datastore.PropertyMap, error) {
	bytes, err := json.Marshal(&n.Notifications)
	if err != nil {
		return nil, err
	}
	n.EncodedNotifications = bytes
	return datastore.GetPLS(n).Save(withMeta)
}
