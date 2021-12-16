// Copyright 2021 The LUCI Authors.
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
	"regexp"
	"strings"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/validation"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

var (
	authGroupNameRegex = regexp.MustCompile(`^([a-z\-]+/)?[0-9a-z_\-\.@]{1,100}$`)
	bucketRegex        = regexp.MustCompile(`^[a-z0-9\-_.]{1,100}$`)
)

// validateProjectCfg implements validation.Func and validates the content of
// Buildbucket project config file.
//
// Validation errors are returned via validation.Context. Non-validation errors
// are directly returned.
func validateProjectCfg(ctx *validation.Context, configSet, path string, content []byte) error {
	cfg := pb.BuildbucketCfg{}
	if err := prototext.Unmarshal(content, &cfg); err != nil {
		ctx.Errorf("invalid BuildbucketCfg proto message: %s", err)
		return nil
	}

	globalCfg, err := GetSettingsCfg(ctx.Context)
	if err != nil {
		// This error is unrelated to the data being validated. So directly return
		// to instruct config service to retry.
		return errors.Annotate(err, "error fetching service config").Err()
	}
	wellKnownExperiments := protoutil.WellKnownExperiments(globalCfg)

	if len(cfg.AclSets) > 0 {
		ctx.Errorf("acl_sets is not allowed any more, use go/lucicfg")
	}

	if len(cfg.BuilderMixins) != 0 {
		ctx.Errorf("builder_mixins is not allowed any more, use go/lucicfg")
	}

	// The format of configSet here is "projects/.*"
	project := strings.Split(configSet, "/")[1]
	bucketNames := stringset.New(len(cfg.Buckets))
	for i, bucket := range cfg.Buckets {
		ctx.Enter("buckets #%d - %s", i, bucket.Name)
		switch err := validateBucketName(bucket.Name, project); {
		case err != nil:
			ctx.Errorf("invalid name %q: %s", bucket.Name, err)
		case bucketNames.Has(bucket.Name):
			ctx.Errorf("duplicate bucket name %q", bucket.Name)
		case i > 0 && strings.Compare(bucket.Name, cfg.Buckets[i-1].Name) < 0:
			ctx.Warningf("bucket %q out of order", bucket.Name)
		}
		bucketNames.Add(bucket.Name)
		validateAcls(ctx, bucket.Acls)
		if len(bucket.AclSets) > 0 {
			ctx.Errorf("acl_sets is not allowed any more, use go/lucicfg")
		}
		if s := bucket.Swarming; s != nil {
			validateProjectSwarming(ctx, s, wellKnownExperiments)
		}
		ctx.Exit()
	}
	return nil
}

// validateProjectSwarming validates project_config.Swarming.
func validateProjectSwarming(ctx *validation.Context, s *pb.Swarming, wellKnownExperiments stringset.Set) {
	ctx.Enter("swarming")
	defer ctx.Exit()

	if s.GetTaskTemplateCanaryPercentage().GetValue() > 100 {
		ctx.Errorf("task_template_canary_percentage.value must must be in [0, 100]")
	}
	if builderDefaults := s.BuilderDefaults; builderDefaults != nil {
		ctx.Errorf("builder_defaults is not allowed anymore")
	}

	builderNames := stringset.New(len(s.Builders))
	for i, b := range s.Builders {
		ctx.Enter("builders #%d", i)
		validateBuilderCfg(ctx, b, wellKnownExperiments)
		if builderNames.Has(b.Name) {
			ctx.Errorf("name: duplicate")
		} else {
			builderNames.Add(b.Name)
		}
		ctx.Exit()
	}
}

func validateBucketName(bucket, project string) error {
	switch {
	case bucket == "":
		return errors.New("bucket name is not specified")
	case strings.HasPrefix(bucket, "luci.") && !strings.HasPrefix(bucket, fmt.Sprintf("luci.%s.", project)):
		return errors.Reason("must start with 'luci.%s.' because it starts with 'luci.' and is defined in the %q project", project, project).Err()
	case !bucketRegex.MatchString(bucket):
		return errors.Reason("%q does not match %q", bucket, bucketRegex).Err()
	}
	return nil
}

func validateAcls(ctx *validation.Context, acls []*pb.Acl) {
	for i, acl := range acls {
		ctx.Enter("acls #%d", i)
		switch {
		case acl.Group != "" && acl.Identity != "":
			ctx.Errorf("either group or identity must be set, not both")
		case acl.Group == "" && acl.Identity == "":
			ctx.Errorf("group or identity must be set")
		case acl.Group != "" && !authGroupNameRegex.MatchString(acl.Group):
			ctx.Errorf("invalid group: %s", acl.Group)
		case acl.Identity != "":
			identityStr := acl.Identity
			if !strings.Contains(acl.Identity, ":") {
				identityStr = fmt.Sprintf("user:%s", acl.Identity)
			}
			if err := identity.Identity(identityStr).Validate(); err != nil {
				ctx.Errorf("%q invalid: %s", acl.Identity, err)
			}
		}
		ctx.Exit()
	}
}

// validateBuilderCfg validate a Builder config message.
func validateBuilderCfg(ctx *validation.Context, b *pb.Builder, wellKnownExperiments stringset.Set) {
	// TODO(yuanjunh): validate builder config.
}
