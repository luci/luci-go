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
	html "html/template"
	"net/mail"
	"regexp"
	text "text/template"

	"github.com/golang/protobuf/proto"

	// Import to register {appid} in validation.Rules
	_ "go.chromium.org/luci/config/appengine/gaeconfig"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/config/validation"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
)

// notifierNameRegexp is a regexp for notifier names.
var notifierNameRegexp = regexp.MustCompile(`^[a-z0-9\-]+$`)

// init registers validators for the project config and email template files.
func init() {
	validation.Rules.Add(
		"regex:projects/.*",
		"${appid}.cfg",
		func(ctx *validation.Context, configSet, path string, content []byte) error {
			cfg := &notifypb.ProjectConfig{}
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
	badRepoURLError       = "repo url %q is invalid"
	duplicateBuilderError = "builder %q is specified more than once in file"
)

// validateNotification is a helper function for validateConfig which validates
// an individual notification configuration.
func validateNotification(c *validation.Context, cfgNotification *notifypb.Notification) {
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
func validateBuilder(c *validation.Context, cfgBuilder *notifypb.Builder, builderNames stringset.Set) {
	if cfgBuilder.Bucket == "" {
		c.Errorf(requiredFieldError, "bucket")
	}
	if cfgBuilder.Name == "" {
		c.Errorf(requiredFieldError, "name")
	}
	if cfgBuilder.Repository != "" {
		if err := gitiles.ValidateRepoURL(cfgBuilder.Repository); err != nil {
			c.Errorf(badRepoURLError, cfgBuilder.Repository)
		}
	}
	fullName := fmt.Sprintf("%s/%s", cfgBuilder.Bucket, cfgBuilder.Name)
	if !builderNames.Add(fullName) {
		c.Errorf(duplicateBuilderError, fullName)
	}
}

// validateNotifier validates a Notifier.
func validateNotifier(c *validation.Context, cfgNotifier *notifypb.Notifier, builderNames stringset.Set) {
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
func validateProjectConfig(ctx *validation.Context, projectCfg *notifypb.ProjectConfig) {
	builderNames := stringset.New(len(projectCfg.Notifiers)) // At least one builder per notifier
	for i, cfgNotifier := range projectCfg.Notifiers {
		ctx.Enter("notifier #%d", i+1)
		validateNotifier(ctx, cfgNotifier, builderNames)
		ctx.Exit()
	}
}

// validateSettings returns an error if the service configuration violates any
// of the requirements in the proto definition.
func validateSettings(ctx *validation.Context, settings *notifypb.Settings) {
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
	if err != nil {
		ctx.Error(err)
	} else {
		// Note: Parse does not return an error if the template attempts to
		// call an undefined template, e.g. {{template "does-not-exist"}}
		if _, err = text.New("subject").Funcs(EmailTemplateFuncs).Parse(subject); err != nil {
			ctx.Error(err) // error includes template name
		}
		// Due to luci-config limitation, we cannot detect an invalid reference to
		// a sub-template defined in a different file.
		if _, err = html.New("body").Funcs(EmailTemplateFuncs).Parse(body); err != nil {
			ctx.Error(err) // error includes template name
		}
	}
	return nil
}
