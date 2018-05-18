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

	// Import to register {appid} in validation.Rules
	_ "go.chromium.org/luci/config/appengine/gaeconfig"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/config/validation"

	notifyConfig "go.chromium.org/luci/luci_notify/api/config"
)

// notifierNameRegexp is a regexp for notifier names.
var notifierNameRegexp = regexp.MustCompile(`^[a-z0-9\-]+$`)

// init registers validators for the project config and email template files.
func init() {
	validation.Rules.Add(
		"regex:projects/.*",
		"${appid}.cfg",
		func(ctx *validation.Context, configSet, path string, content []byte) error {
			cfg := &notifyConfig.ProjectConfig{}
			if err := proto.UnmarshalText(string(content), cfg); err != nil {
				ctx.Errorf("invalid ProjectConfig proto message: %s", err)
			} else {
				validateProjectConfig(ctx, cfg)
			}
			return nil
		})

	validation.Rules.Add(
		"regex:projects/.*",
		`regex:${appid}/email-templates/[^/]+\.template`,
		validateEmailTemplateFile)
}

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
func validateProjectConfig(ctx *validation.Context, projectCfg *notifyConfig.ProjectConfig) {
	notifierNames := stringset.New(len(projectCfg.Notifiers))
	builderNames := stringset.New(len(projectCfg.Notifiers)) // At least one builder per notifier
	for i, cfgNotifier := range projectCfg.Notifiers {
		ctx.Enter("notifier #%d", i+1)
		validateNotifier(ctx, cfgNotifier, notifierNames, builderNames)
		ctx.Exit()
	}
}

// validateSettings returns an error if the service configuration violates any
// of the requirements in the proto definition.
func validateSettings(ctx *validation.Context, settings *notifyConfig.Settings) {
	switch {
	case settings.MiloHost == "":
		ctx.Errorf(requiredFieldError, "milo_host")
	case validation.ValidateHostname(settings.MiloHost) != nil:
		ctx.Errorf(invalidFieldError, "milo_host")
	}
}

// validateEmailTemplateFile validates an email template file, including
// its filename and contents.
func validateEmailTemplateFile(ctx *validation.Context, configSet, path string, content []byte) error {
	// Validate file name.
	rgx := emailTemplateFilenameRegexp(ctx.Context)
	if !rgx.MatchString(path) {
		ctx.Errorf("filename does not match %q", rgx.String())
	}

	// Validate file contents.
	subject, body, err := splitEmailTemplateFile(string(content))
	var t ParsedEmailTemplate
	if err != nil {
		ctx.Error(err)
	} else if err := t.Parse(subject, body); err != nil {
		ctx.Error(err)
	}
	return nil
}
