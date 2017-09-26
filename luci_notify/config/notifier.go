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
	"fmt"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
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
}

func newNotificationConfig(cfg *notifyConfig.Notification) NotificationConfig {
	var recipients []string
	if cfg.Email != nil {
		recipients = cfg.Email.Recipients
	}
	return NotificationConfig{
		OnSuccess:       cfg.OnSuccess,
		OnFailure:       cfg.OnFailure,
		OnChange:        cfg.OnChange,
		EmailRecipients: recipients,
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

	// Notifications is a list of notification configurations.
	Notifications []NotificationConfig `gae:"-"`
}

// NewNotifier constructs a new Notifier from a parent key and a Notifier
// as defined by the project configuration proto.
func NewNotifier(parent *datastore.Key, cfg *notifyConfig.Notifier) *Notifier {
	var builderIDs []string
	for _, b := range cfg.Builders {
		if b.Name != "" {
			id := fmt.Sprintf("buildbucket/%s/%s", b.Bucket, b.Name)
			builderIDs = append(builderIDs, id)
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
		Notifications: notifications,
	}
}

// LookupNotifiers retrieves all notifiers from the datastore for a particular builder ID.
func LookupNotifiers(c context.Context, build *buildbucket.BuildInfo) ([]*Notifier, error) {
	var notifiers []*Notifier
	query := datastore.NewQuery("Notifier").Eq("BuilderIDs", build.BuilderID())
	err := datastore.GetAll(c, query, &notifiers)
	return notifiers, err
}

// Load loads a Notifier's information from props.
//
// This implements PropertyLoadSaver. Load decodes the property Notifications
// stored in the datastore which is encoded json, and decodes it into the
// struct's Notifications field.
func (n *Notifier) Load(props datastore.PropertyMap) error {
	if pdata, ok := props["Notifications"]; ok {
		configs := pdata.Slice()
		if len(configs) != 1 {
			return fmt.Errorf("property `Notifications` is a property slice")
		}
		configBytes, ok := configs[0].Value().([]byte)
		if !ok {
			return fmt.Errorf("expected byte array for property `Notifications`")
		}
		if err := json.Unmarshal(configBytes, &n.Notifications); err != nil {
			return err
		}
		delete(props, "Notifications")
	}
	return datastore.GetPLS(n).Load(props)
}

// Save saves a Notifier's information to a property map.
//
// This implements PropertyLoadSaver. Save encodes the Notifications
// field as json and stores it in the Notifications property.
func (n *Notifier) Save(withMeta bool) (datastore.PropertyMap, error) {
	props, err := datastore.GetPLS(n).Save(withMeta)
	if err != nil {
		return nil, err
	}
	bytes, err := json.Marshal(&n.Notifications)
	if err != nil {
		return nil, err
	}
	props["Notifications"] = datastore.MkProperty(bytes)
	return props, nil
}
