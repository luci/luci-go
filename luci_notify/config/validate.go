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

const (
	requiredFieldError = "field %q is required"
	invalidFieldError  = "field %q has invalid format"
	uniqueFieldError   = "field %q must be unique in %s"
	badEmailError      = "recipient %q is not a valid RFC 5322 email address"
)

// extractProjectConfig attempts to unmarshal the configuration out of the
// luci-config message and then validate it.
func extractProjectConfig(cfg *configInterface.Config) (*notifyConfig.ProjectConfig, error) {
	project := notifyConfig.ProjectConfig{}
	if err := proto.UnmarshalText(cfg.Content, &project); err != nil {
		err = errors.Annotate(err, "unmarshalling proto").Err()
		return nil, err
	}
	if err := validateProjectConfig(cfg.ConfigSet, &project); err != nil {
		err = errors.Annotate(err, "validating config").Err()
		return nil, err
	}
	return &project, nil
}

// validateNotification is a helper function for validateConfig which validates
// an individual notification configuration.
func validateNotification(c *validation.Context, cfgNotification *notifyConfig.Notification) {
	if cfgNotification.Email != nil {
		for _, addr := range cfgNotification.Email.Recipients {
			if _, err := mail.ParseAddress(addr); err != nil {
				c.Error(badEmailError, addr)
			}
		}
	}
}

// validateBuilder is a helper function for validateConfig which validates
// an individual Builder.
func validateBuilder(c *validation.Context, cfgBuilder *notifyConfig.Builder) {
	if cfgBuilder.Bucket == "" {
		c.Error(requiredFieldError, "bucket")
	}
	if cfgBuilder.Name == "" {
		c.Error(requiredFieldError, "name")
	}
}

// validateNotifier is a helper function for validateConfig which validates
// a Notifier.
func validateNotifier(c *validation.Context, cfgNotifier *notifyConfig.Notifier, notifierNames stringset.Set) {
	switch {
	case cfgNotifier.Name == "":
		c.Error(requiredFieldError, "name")
	case !notifierNameRegexp.MatchString(cfgNotifier.Name):
		c.Error(invalidFieldError, "name")
	case !notifierNames.Add(cfgNotifier.Name):
		c.Error(uniqueFieldError, "name", "project")
	}
	for i, cfgNotification := range cfgNotifier.Notifications {
		c.Enter("notification #%d", i+1)
		validateNotification(c, cfgNotification)
		c.Exit()
	}
	for i, cfgBuilder := range cfgNotifier.Builders {
		c.Enter("builder #%d", i+1)
		validateBuilder(c, cfgBuilder)
		c.Exit()
	}
}

// validateProjectConfig returns an error if the configuration violates any of the
// requirements in the proto definition.
func validateProjectConfig(configName string, projectCfg *notifyConfig.ProjectConfig) error {
	c := &validation.Context{}
	c.SetFile(configName)
	notifierNames := stringset.New(len(projectCfg.Notifiers))
	for i, cfgNotifier := range projectCfg.Notifiers {
		c.Enter("notifier #%d", i+1)
		validateNotifier(c, cfgNotifier, notifierNames)
		c.Exit()
	}
	return c.Finalize()
}

// extractSettings attempts to unmarshal a service-level configuration into a
// specified location, and then tries to validate it.
func extractSettings(cfg *configInterface.Config) (*Settings, error) {
	settings := Settings{Revision: cfg.Revision}
	err := proto.UnmarshalText(cfg.Content, &settings.Settings)
	if err != nil {
		return nil, errors.Annotate(err, "unmarshalling proto").Err()
	}
	if err := validateSettings(&settings.Settings); err != nil {
		return nil, errors.Annotate(err, "validating settings").Err()
	}
	return &settings, nil
}

// validateSettings returns an error if the service configuration violates any
// of the requirements in the proto definition.
func validateSettings(settings *notifyConfig.Settings) error {
	c := &validation.Context{}
	c.SetFile("settings.cfg")
	switch {
	case settings.MiloHost == "":
		c.Error(requiredFieldError, "milo_host")
	case validation.ValidateHostname(settings.MiloHost) != nil:
		c.Error(invalidFieldError, "milo_host")
	}
	return c.Finalize()
}
