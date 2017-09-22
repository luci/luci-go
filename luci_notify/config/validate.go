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
	"net/mail"
	"regexp"

	"github.com/golang/protobuf/proto"

	configInterface "go.chromium.org/luci/common/config"
	"go.chromium.org/luci/common/config/validation"
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

// validateNotification is a helper function for validateConfig which validates
// an individual notification configuration.
func validateNotification(c *validation.Context, cfgNotification *notifyConfig.Notification) {
	c.Enter("notification")
	defer c.Exit()

	if cfgNotification.Email != nil {
		for _, addr := range cfgNotification.Email.Recipients {
			if _, err := mail.ParseAddress(addr); err != nil {
				c.Error("recipient %q is not a valid RFC 5322 email address", addr)
			}
		}
	}
}

// validateBuilder is a helper function for validateConfig which validates
// an individual Builder.
func validateBuilder(c *validation.Context, cfgBuilder *notifyConfig.Builder) {
	if cfgBuilder.Bucket == "" {
		c.Error("field `Bucket` in builder is required")
	}
	if cfgBuilder.Name == "" {
		c.Error("field `Name` in builder is required")
	}
}

// validateNotifier is a helper function for validateConfig which validates
// a Notifier.
func validateNotifier(c *validation.Context, cfgNotifier *notifyConfig.Notifier, notifierNames stringset.Set) {
	c.Enter("notifier %q", cfgNotifier.Name)
	defer c.Exit()

	switch {
	case cfgNotifier.Name == "":
		c.Error("field `Name` is required")
	case !notifierNameRegexp.MatchString(cfgNotifier.Name):
		c.Error("value %q of field `Name` has invalid format", cfgNotifier.Name)
	case !notifierNames.Add(cfgNotifier.Name):
		c.Error("value %q of field `Name` must be unique in project", cfgNotifier.Name)
	}
	for _, cfgNotification := range cfgNotifier.Notifications {
		validateNotification(c, cfgNotification)
	}
	for _, cfgBuilder := range cfgNotifier.Builders {
		validateBuilder(c, cfgBuilder)
	}
}

// validateConfig returns an error if the configuration violates any of the
// requirements in the proto definition.
func validateConfig(projectCfg *notifyConfig.ProjectConfig) error {
	c := &validation.Context{}
	notifierNames := stringset.New(len(projectCfg.Notifiers))
	for _, cfgNotifier := range projectCfg.Notifiers {
		validateNotifier(c, cfgNotifier, notifierNames)
	}
	return c.Finalize()
}
