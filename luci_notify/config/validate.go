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
	"fmt"
	"net/mail"
	"regexp"

	"github.com/golang/protobuf/proto"

	configInterface "go.chromium.org/luci/common/config"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	notifyConfig "go.chromium.org/luci/luci_notify/api/config"
)

// Regexp for notifier names.
var notifierNameRegexp = regexp.MustCompile("^[a-z-]+$")

// extractConfig attempts to unmarshal the configuration out of the
// luci-config message and then validate it.
func extractConfig(cfg *configInterface.Config) (*notifyConfig.ProjectConfig, error) {
	project := notifyConfig.ProjectConfig{}
	if err := proto.UnmarshalText(cfg.Content, &project); err != nil {
		err = errors.Annotate(err, "unmarshalling proto").Err()
		return nil, err
	}
	if err := validateConfig(&project); err != nil {
		err = errors.Annotate(err, "validating config").Err()
		return nil, err
	}
	return &project, nil
}

// validateNotification is a helper function to validateConfig which validates
// an individual notification configuration.
func validateNotification(cfgNotification *notifyConfig.Notification) error {
	if cfgNotification.Email != nil {
		for _, addr := range cfgNotification.Email.Recipients {
			if _, err := mail.ParseAddress(addr); err != nil {
				return errors.Annotate(err,
					"recipient is not a valid RFC 5322 email address").Err()
			}
		}
	}
	return nil
}

// validateConfig validates the given configuration, returning an error if the
// configuration violates any of the requirements in the proto definition.
func validateConfig(projectCfg *notifyConfig.ProjectConfig) error {
	notifierNames := stringset.New(len(projectCfg.Notifiers))
	for _, cfgNotifier := range projectCfg.Notifiers {
		if cfgNotifier.Name == "" {
			return fmt.Errorf("field `name` in notifier is required")
		}
		if !notifierNameRegexp.MatchString(cfgNotifier.Name) {
			return fmt.Errorf("value `%s` of field `name` in notifier has invalid format", cfgNotifier.Name)
		}
		if notifierNames.Has(cfgNotifier.Name) {
			return fmt.Errorf("value `%s` of field `name` in notifier must be unique in project", cfgNotifier.Name)
		}

		// Validate notifications.
		for _, cfgNotification := range cfgNotifier.Notifications {
			if err := validateNotification(cfgNotification); err != nil {
				return err
			}
		}

		// Validate builders.
		for _, cfgBuilder := range cfgNotifier.Builders {
			if cfgBuilder.Name == "" {
				return fmt.Errorf("field `name` in builder is required")
			}
		}
	}
	return nil
}
