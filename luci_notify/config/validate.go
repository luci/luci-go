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
	"strings"
	text "text/template"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/config/validation"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/mailtmpl"
)

// TreeNameRE matches valid LUCI Tree Status tree names.
var TreeNameRE = regexp.MustCompile("^[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$")

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
}

const (
	requiredFieldError      = "field %q is required"
	invalidFieldError       = "field %q has invalid format"
	uniqueFieldError        = "field %q must be unique in %s"
	badEmailError           = "recipient %q is not a valid RFC 5322 email address"
	badRepoURLError         = "repo url %q is invalid"
	duplicateBuilderError   = "builder %q is specified more than once in file"
	duplicateTreeNameError  = "builder has multiple tree closers with tree name %q"
	badRegexError           = "field %q contains an invalid regex: %s"
	onlyTreeNameOrHostError = "only one of tree_name and tree_status_host may be assigned"
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

	validateRegexField(c, "failed_step_regexp", cfgNotification.FailedStepRegexp)
	validateRegexField(c, "failed_step_regexp_exclude", cfgNotification.FailedStepRegexpExclude)
}

// validateTreeCloser is a helper function for validateConfig which validates an
// individual tree closer configuration.
func validateTreeCloser(c *validation.Context, cfgTreeCloser *notifypb.TreeCloser, uniqueTreeNames stringset.Set) {
	logicalTreeName := cfgTreeCloser.TreeName
	if logicalTreeName == "" && cfgTreeCloser.TreeStatusHost != "" {
		logicalTreeName = TreeNameFromHost(cfgTreeCloser.TreeStatusHost)
	}
	if cfgTreeCloser.TreeName != "" && cfgTreeCloser.TreeStatusHost != "" {
		c.Errorf(onlyTreeNameOrHostError)
	}

	if logicalTreeName == "" {
		c.Errorf(requiredFieldError, "tree_name")
	} else if !TreeNameRE.MatchString(logicalTreeName) {
		// Allocate the error to whichever of (tree_name, tree_status_host) was assigned.
		if cfgTreeCloser.TreeName != "" {
			c.Errorf(invalidFieldError, "tree_name")
		} else {
			c.Errorf(invalidFieldError, "tree_status_host")
		}
	}

	if !uniqueTreeNames.Add(logicalTreeName) {
		c.Errorf(duplicateTreeNameError, logicalTreeName)
	}

	validateRegexField(c, "failed_step_regexp", cfgTreeCloser.FailedStepRegexp)
	validateRegexField(c, "failed_step_regexp_exclude", cfgTreeCloser.FailedStepRegexpExclude)
}

// validateRegexField validates that a field contains a valid regex.
func validateRegexField(c *validation.Context, fieldName, regex string) {
	_, err := regexp.Compile(regex)
	if err != nil {
		c.Errorf(badRegexError, fieldName, err.Error())
	}
}

// validateBuilder is a helper function for validateConfig which validates
// an individual Builder.
func validateBuilder(c *validation.Context, cfgBuilder *notifypb.Builder, builderNames stringset.Set) {
	if cfgBuilder.Bucket == "" {
		c.Errorf(requiredFieldError, "bucket")
	}
	if strings.HasPrefix(cfgBuilder.Bucket, "luci.") {
		// TODO(tandrii): change to warning once our validation library supports it.
		c.Errorf(`field "bucket" should not include legacy "luci.<project_name>." prefix, given %q`, cfgBuilder.Bucket)
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
	uniqueTreeNames := stringset.New(len(cfgNotifier.TreeClosers))
	for i, cfgTreeCloser := range cfgNotifier.TreeClosers {
		c.Enter("tree_closer #%d", i+1)
		validateTreeCloser(c, cfgTreeCloser, uniqueTreeNames)
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
	if settings.LuciTreeStatusHost != "" {
		if validation.ValidateHostname(settings.LuciTreeStatusHost) != nil {
			ctx.Errorf(invalidFieldError, "luci_tree_status_host")
		}
	}
}

// validateEmailTemplateFile validates an email template file, including
// its filename and contents.
func validateEmailTemplateFile(ctx *validation.Context, configSet, path string, content []byte) error {
	// Validate file name.
	rgx, err := emailTemplateFilenameRegexp(ctx.Context)
	if err != nil {
		return err
	}

	if !rgx.MatchString(path) {
		ctx.Errorf("filename does not match %q", rgx.String())
	}

	// Validate file contents.
	subject, body, err := mailtmpl.SplitTemplateFile(string(content))
	if err != nil {
		ctx.Error(err)
	} else {
		// Note: Parse does not return an error if the template attempts to
		// call an undefined template, e.g. {{template "does-not-exist"}}
		if _, err = text.New("subject").Funcs(mailtmpl.Funcs).Parse(subject); err != nil {
			ctx.Error(err) // error includes template name
		}
		// Due to luci-config limitation, we cannot detect an invalid reference to
		// a sub-template defined in a different file.
		if _, err = html.New("body").Funcs(mailtmpl.Funcs).Parse(body); err != nil {
			ctx.Error(err) // error includes template name
		}
	}
	return nil
}
