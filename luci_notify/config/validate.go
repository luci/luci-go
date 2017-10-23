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
	"net/http"
	"net/mail"
	"regexp"

	"golang.org/x/net/context"

	bbapi "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/config/validation"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	notifyConfig "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/server/auth"
)

// Regexp for notifier names.
var notifierNameRegexp = regexp.MustCompile("^[a-z-]+$")

const (
	requiredFieldError = "field %q is required"
	invalidFieldError  = "field %q has invalid format"
	uniqueFieldError   = "field %q must be unique in %s"
	badEmailError      = "recipient %q is not a valid RFC 5322 email address"
)

// BucketIndex represents an mapping of buildbucket buckets to project IDs.
type BucketIndex interface {
	GetProject(c context.Context, bucket string) (string, error)
}

// BucketIndexImpl is an implemention of a BucketIndex.
type BucketIndexImpl struct {
	// Host is the hostname of the buildbucket isntace associated with this
	// luci-notify instance. It is used to ask buildbucket for the project_id
	// associated with a bucket. It is derived from Settings.
	Host string

	// cache is a map of bucket names to project IDs. This is to prevent
	// making too many RPC calls to buildbucket since builders will likely
	// share buckets.
	Cache map[string]string
}

// GetProject looks up the project ID associated with a buildbucket bucket.
func (b *BucketIndexImpl) GetProject(c context.Context, bucket string) (string, error) {
	if proj, ok := b.Cache[bucket]; ok {
		return proj, nil
	}
	transport, err := auth.GetRPCTransport(c, auth.AsSelf)
	if err != nil {
		return "", errors.Annotate(err, "getting transport").Err()
	}
	svc, err := bbapi.New(&http.Client{Transport: transport})
	if err != nil {
		return "", errors.Annotate(err, "creating service").Err()
	}
	svc.BasePath = fmt.Sprintf("https://%s/_ah/api/buildbucket/v1/", b.Host)
	result, err := svc.GetBucket(bucket).Fields("project_id").Context(c).Do()
	if err != nil {
		return "", errors.Annotate(err, "making request").Err()
	}
	b.Cache[bucket] = result.ProjectId
	return result.ProjectId, nil
}

type projectValidationContext struct {
	validation.Context
	BucketIndex

	// gContext is the external calling context used for API calls.
	gContext context.Context

	// Project is the project associated with the config being validated.
	project string

	// NotifierNames is a set of notifier names to detect whether the names
	// conflict.
	notifierNames stringset.Set
}

// validateNotification is a helper function for validateConfig which validates
// an individual notification configuration.
func validateNotification(c *projectValidationContext, cfgNotification *notifyConfig.Notification) {
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
func validateBuilder(c *projectValidationContext, cfgBuilder *notifyConfig.Builder) {
	if cfgBuilder.Bucket == "" {
		c.Error(requiredFieldError, "bucket")
	} else {
		bucketProject, err := c.GetProject(c.gContext, cfgBuilder.Bucket)
		if err != nil {
			c.Error(err.Error())
		}
		if c.project != bucketProject {
			c.Error("bucket %q is not part of project %q", cfgBuilder.Bucket, c.project)
		}
	}
	if cfgBuilder.Name == "" {
		c.Error(requiredFieldError, "name")
	}
}

// validateNotifier is a helper function for validateConfig which validates
// a Notifier.
func validateNotifier(c *projectValidationContext, cfgNotifier *notifyConfig.Notifier) {
	switch {
	case cfgNotifier.Name == "":
		c.Error(requiredFieldError, "name")
	case !notifierNameRegexp.MatchString(cfgNotifier.Name):
		c.Error(invalidFieldError, "name")
	case !c.notifierNames.Add(cfgNotifier.Name):
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
func validateProjectConfig(g context.Context, projectName string, projectCfg *notifyConfig.ProjectConfig, index BucketIndex) error {
	c := &projectValidationContext{
		BucketIndex:   index,
		gContext:      g,
		project:       projectName,
		notifierNames: stringset.New(len(projectCfg.Notifiers)),
	}
	c.SetFile(projectName)
	for i, cfgNotifier := range projectCfg.Notifiers {
		c.Enter("notifier #%d", i+1)
		validateNotifier(c, cfgNotifier)
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
		c.Error(requiredFieldError, "milo_host")
	case validation.ValidateHostname(settings.MiloHost) != nil:
		c.Error(invalidFieldError, "milo_host")
	}
	switch {
	case settings.BuildbucketHost == "":
		c.Error(requiredFieldError, "buildbucket_host")
	case validation.ValidateHostname(settings.BuildbucketHost) != nil:
		c.Error(invalidFieldError, "buildbucket_host")
	}
	return c.Finalize()
}
