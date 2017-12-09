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

	"go.chromium.org/luci/common/config/validation"
	"go.chromium.org/luci/common/data/stringset"
	notifyConfig "go.chromium.org/luci/luci_notify/api/config"
)

// Regexp for notifier names.
var notifierNameRegexp = regexp.MustCompile("^[a-z-]+$")

const (
	requiredFieldError    = "field %q is required"
	invalidFieldError     = "field %q has invalid format"
	uniqueFieldError      = "field %q must be unique in %s"
	badEmailError         = "recipient %q is not a valid RFC 5322 email address"
	duplicateBuilderError = "builder %q is specified more than once in file"
)

// validateNotification is a helper function for validateConfig which validates
// an individual notification configuration.
func validateNotification(c *validation.Context, cfgNotification *notifyConfig.Notification) {
	if cfgNotification.Email != nil {
		for _, addr := range cfgNotification.Email.Recipients {
			if _, err := mail.ParseAddress(addr); err != nil {
				c.Errorf(badEmailError, addr)
			}
		}
	}
}

// validateBuilder is a helper function for validateConfig which validates
// an individual Builder.
func validateBuilder(c *validation.Context, cfgBuilder *notifyConfig.Builder, builderNames stringset.Set) {
	if cfgBuilder.Bucket == "" {
		c.Errorf(requiredFieldError, "bucket")
	}
	if cfgBuilder.Name == "" {
		c.Errorf(requiredFieldError, "name")
	}
	fullName := fmt.Sprintf("%s/%s", cfgBuilder.Bucket, cfgBuilder.Name)
	if !builderNames.Add(fullName) {
		c.Errorf(duplicateBuilderError, fullName)
	}
}

// validateNotifier is a helper function for validateConfig which validates
// a Notifier.
func validateNotifier(c *validation.Context, cfgNotifier *notifyConfig.Notifier, notifierNames, builderNames stringset.Set) {
	switch {
	case cfgNotifier.Name == "":
		c.Errorf(requiredFieldError, "name")
	case !notifierNameRegexp.MatchString(cfgNotifier.Name):
		c.Errorf(invalidFieldError, "name")
	case !notifierNames.Add(cfgNotifier.Name):
		c.Errorf(uniqueFieldError, "name", "project")
	}
	for i, cfgNotification := range cfgNotifier.Notifications {
		c.Enter("notification #%d", i+1)
		validateNotification(c, cfgNotification)
		c.Exit()
	}
	for i, cfgBuilder := range cfgNotifier.Builders {
		c.Enter("builder #%d", i+1)
		validateBuilder(c, cfgBuilder, builderNames)
		c.Exit()
	}
}

// validateProjectConfig returns an error if the configuration violates any of the
// requirements in the proto definition.
func validateProjectConfig(configName string, projectCfg *notifyConfig.ProjectConfig) error {
	c := &validation.Context{}
	c.SetFile(configName)
	notifierNames := stringset.New(len(projectCfg.Notifiers))
	builderNames := stringset.New(len(projectCfg.Notifiers)) // At least one builder per notifier
	for i, cfgNotifier := range projectCfg.Notifiers {
		c.Enter("notifier #%d", i+1)
		validateNotifier(c, cfgNotifier, notifierNames, builderNames)
		c.Exit()
	}
	return c.Finalize()
}

// validateSettings returns an error if the service configuration violates any
// of the requirements in the proto definition.
func validateSettings(settings *notifyConfig.Settings) error {
	c := &validation.Context{}
	c.SetFile("settings.cfg")
	switch {
	case settings.MiloHost == "":
		c.Errorf(requiredFieldError, "milo_host")
	case validation.ValidateHostname(settings.MiloHost) != nil:
		c.Errorf(invalidFieldError, "milo_host")
	}
	return c.Finalize()
}
