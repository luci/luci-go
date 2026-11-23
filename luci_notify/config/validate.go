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
	"context"
	"fmt"
	html "html/template"
	"net/http"
	"net/mail"
	"regexp"
	"slices"
	"strings"
	text "text/template"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/lhttp"
	luciconfig "go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	tspb "go.chromium.org/luci/tree_status/proto/v1"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/mailtmpl"
)

// TreeNameRE matches valid LUCI Tree Status tree names.
var TreeNameRE = regexp.MustCompile("^[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$")

type treeGetter func(ctx context.Context, treeName string) (*tspb.Tree, error)

type treeGetterKeyType struct{}

var treeGetterKey = treeGetterKeyType{}

func withTreeGetter(ctx context.Context, getter treeGetter) context.Context {
	return context.WithValue(ctx, treeGetterKey, getter)
}

func newTreeGetterFromSettings(ctx context.Context) (treeGetter, error) {
	settings, err := FetchSettings(ctx)
	if err != nil {
		return nil, errors.Fmt("fetching service settings for tree validation: %w", err)
	}
	if settings.LuciTreeStatusHost == "" {
		return nil, errors.New("luci_tree_status_host is not configured in settings.cfg")
	}
	transport, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return nil, errors.Fmt("getting RPC transport for tree validation: %w", err)
	}
	rpcOpts := prpc.DefaultOptions()
	rpcOpts.Insecure = lhttp.IsLocalHost(settings.LuciTreeStatusHost)
	rpcOpts.PerRPCTimeout = 10 * time.Second
	prpcClient := &prpc.Client{
		C:       &http.Client{Transport: transport},
		Host:    settings.LuciTreeStatusHost,
		Options: rpcOpts,
	}
	treesClient := tspb.NewTreesPRPCClient(prpcClient)
	return func(ctx context.Context, treeName string) (*tspb.Tree, error) {
		return treesClient.GetTree(ctx, &tspb.GetTreeRequest{
			Name: fmt.Sprintf("trees/%s", treeName),
		}, prpc.ExpectedCode(codes.NotFound, codes.PermissionDenied))
	}, nil
}

func newMemoizedTreeGetter() treeGetter {
	type cachedTree struct {
		tree *tspb.Tree
		err  error
	}
	var base treeGetter
	cache := map[string]cachedTree{}
	return func(c context.Context, treeName string) (*tspb.Tree, error) {
		if entry, ok := cache[treeName]; ok {
			return entry.tree, entry.err
		}
		if base == nil {
			if override, ok := c.Value(treeGetterKey).(treeGetter); ok && override != nil {
				base = override
			} else {
				var err error
				base, err = newTreeGetterFromSettings(c)
				if err != nil {
					return nil, err
				}
			}
		}
		tree, err := base(c, treeName)
		cache[treeName] = cachedTree{tree: tree, err: err}
		return tree, err
	}
}

// init registers the validator for project config files.
func init() {
	validation.Rules.Add("regex:projects/.*", "${appid}.cfg", validateProjectConfigFile)
}

func validateProjectConfigFile(ctx *validation.Context, configSet, path string, content []byte) error {
	projectID := luciconfig.Set(configSet).Project()
	if projectID == "" {
		ctx.Errorf("invalid project config set %q", configSet)
		return nil
	}
	cfg := &notifypb.ProjectConfig{}
	if err := proto.UnmarshalText(string(content), cfg); err != nil {
		ctx.Errorf("invalid ProjectConfig proto message: %s", err)
		return nil
	}
	return validateProjectConfig(ctx, projectID, cfg, newMemoizedTreeGetter())
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
	unknownTreeError        = "tree %q is not configured in LUCI Tree Status"
	inaccessibleTreeError   = "LUCI Notify does not have access to tree %q in LUCI Tree Status; grant the LUCI Notify service account role/treestatus.reader (or role/treestatus.limitedReader) in the tree's realms.cfg (when onboarding a new realm-ACL tree, land realms.cfg before luci-notify.cfg), or add it to group \"luci-tree-status-access\" for default-ACL trees"
	unauthorizedTreeError   = "project %q is not authorized to close tree %q; check the tree's \"projects\" list in the LUCI Tree Status service config (config.cfg)"
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
func validateTreeCloser(c *validation.Context, projectID string, cfgTreeCloser *notifypb.TreeCloser, uniqueTreeNames stringset.Set, getTree treeGetter) error {
	logicalTreeName := cfgTreeCloser.TreeName
	if logicalTreeName == "" && cfgTreeCloser.TreeStatusHost != "" {
		logicalTreeName = TreeNameFromHost(cfgTreeCloser.TreeStatusHost)
	}
	bothAssigned := cfgTreeCloser.TreeName != "" && cfgTreeCloser.TreeStatusHost != ""
	if bothAssigned {
		c.Errorf(onlyTreeNameOrHostError)
	}

	validTreeName := false
	if logicalTreeName == "" {
		c.Errorf(requiredFieldError, "tree_name")
	} else if !TreeNameRE.MatchString(logicalTreeName) {
		// Allocate the error to whichever of (tree_name, tree_status_host) was assigned.
		if cfgTreeCloser.TreeName != "" {
			c.Errorf(invalidFieldError, "tree_name")
		} else {
			c.Errorf(invalidFieldError, "tree_status_host")
		}
	} else {
		validTreeName = !bothAssigned
	}

	if !uniqueTreeNames.Add(logicalTreeName) {
		c.Errorf(duplicateTreeNameError, logicalTreeName)
	}

	validateRegexField(c, "failed_step_regexp", cfgTreeCloser.FailedStepRegexp)
	validateRegexField(c, "failed_step_regexp_exclude", cfgTreeCloser.FailedStepRegexpExclude)

	if validTreeName && projectID != "" && getTree != nil {
		tree, err := getTree(c.Context, logicalTreeName)
		if err != nil {
			switch grpcutil.Code(err) {
			case codes.NotFound:
				c.Errorf(unknownTreeError, logicalTreeName)
			case codes.PermissionDenied:
				c.Errorf(inaccessibleTreeError, logicalTreeName)
			default:
				return errors.Fmt("validating tree %q: %w", logicalTreeName, err)
			}
		} else if !slices.Contains(tree.GetProjects(), projectID) {
			c.Errorf(unauthorizedTreeError, projectID, logicalTreeName)
		}
	}
	return nil
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
func validateNotifier(c *validation.Context, projectID string, cfgNotifier *notifypb.Notifier, builderNames stringset.Set, getTree treeGetter) error {
	for i, cfgNotification := range cfgNotifier.Notifications {
		c.Enter("notification #%d", i+1)
		validateNotification(c, cfgNotification)
		c.Exit()
	}
	uniqueTreeNames := stringset.New(len(cfgNotifier.TreeClosers))
	for i, cfgTreeCloser := range cfgNotifier.TreeClosers {
		c.Enter("tree_closer #%d", i+1)
		err := validateTreeCloser(c, projectID, cfgTreeCloser, uniqueTreeNames, getTree)
		c.Exit()
		if err != nil {
			return err
		}
	}
	for i, cfgBuilder := range cfgNotifier.Builders {
		c.Enter("builder #%d", i+1)
		validateBuilder(c, cfgBuilder, builderNames)
		c.Exit()
	}
	return nil
}

// validateProjectConfig records blocking validation errors on ctx if the
// configuration violates any proto or tree-ownership requirements, and returns
// a non-nil error only if an operational failure (such as a transient RPC
// error) prevents validation from completing.
func validateProjectConfig(ctx *validation.Context, projectID string, projectCfg *notifypb.ProjectConfig, getTree treeGetter) error {
	builderNames := stringset.New(len(projectCfg.Notifiers)) // At least one builder per notifier
	for i, cfgNotifier := range projectCfg.Notifiers {
		ctx.Enter("notifier #%d", i+1)
		err := validateNotifier(ctx, projectID, cfgNotifier, builderNames, getTree)
		ctx.Exit()
		if err != nil {
			return err
		}
	}
	return nil
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
