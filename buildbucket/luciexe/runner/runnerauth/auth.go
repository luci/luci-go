// Copyright 2019 The LUCI Authors.
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

package runnerauth

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/authctx"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/lucictx"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// System prepares auth.Authenticator used internally by luciexe.
func System(ctx context.Context, args *pb.RunnerArgs) (*auth.Authenticator, error) {
	// If we are given a system logical account name, use it. Otherwise, we use
	// whatever is the default account now (don't switch to a system one). Happens
	// when launching the runner locally on a workstation. It picks up the
	// developer account.
	if args.LuciSystemAccount != "" {
		var err error
		ctx, err = lucictx.SwitchLocalAccount(ctx, args.LuciSystemAccount)
		if err != nil {
			return nil, errors.Annotate(err, "failed to prepare system auth context").Err()
		}
	}

	// The list of scopes we need here is static, it depends only on RPCs luciexe
	// itself is making.
	authOpts := chromeinfra.DefaultAuthOptions()
	authOpts.Scopes = []string{
		"https://www.googleapis.com/auth/cloud-platform", // for Logdog
		"https://www.googleapis.com/auth/userinfo.email", // for Logdog/Buildbucket
	}

	a := auth.NewAuthenticator(ctx, auth.SilentLogin, authOpts)

	// This verifies we actually have the cached refresh token or fails with
	// auth.ErrLoginRequired if we don't (may also fail with other errors).
	switch email, err := a.GetEmail(); {
	case err == auth.ErrLoginRequired:
		logging.Errorf(ctx,
			"If you're running this locally, please make sure you're logged in with:\n"+
				"  luci-auth login -scopes %q", strings.Join(authOpts.Scopes, " "))
		fallthrough
	case err != nil:
		return nil, errors.Annotate(err, "failed to get system account email").Err()
	default:
		logging.Infof(ctx, "Account used for luciexe RPCs: %s", email)
		return a, nil
	}
}

// UserServers launches various local servers that serve tokens to the user
// executable.
func UserServers(ctx context.Context, args *pb.RunnerArgs) (*authctx.Context, error) {
	// Construct authentication option with the set of scopes to be used through
	// out the runner. This is superset of all scopes we might need. It is more
	// efficient to create a single token with all the scopes than make a bunch
	// of smaller-scoped tokens. We trust Google APIs enough to send widely-scoped
	// tokens to them.
	//
	// Note that the user subprocess are still free to request whatever scopes
	// they need (though LUCI_CONTEXT protocol). The scopes here are actually used
	// only internally by the runner (e.g. in the local token server for Firebase)
	// or when running luciexe on a workstation under some end-user account (in
	// this case we can only generate tokens for which there exist a cached
	// refresh token setup via `luci-auth login -scopes ...`, scopes requested
	// through LUCI_CONTEXT protocol are ignored in this case).
	//
	// See https://developers.google.com/identity/protocols/googlescopes for the
	// list of available scopes.
	authOpts := chromeinfra.DefaultAuthOptions()
	authOpts.Scopes = []string{
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/userinfo.email",
	}

	flags := args.GetBuild().GetInput().GetProperties().GetFields()["$kitchen"].GetStructValue().GetFields()
	isEnabled := func(key string, defaultValue bool) bool {
		v := flags[key]
		if v == nil {
			return defaultValue
		}
		return v.GetBoolValue()
	}

	user := &authctx.Context{
		ID:            "task",
		Options:       authOpts,
		EnableGitAuth: isEnabled("git_auth", true),
		// GCE emulation is a superset of deprecated devshell method. If devshell
		// was explicitly disabled on a builder before, we need to disable GCE
		// emulation as well.
		EnableGCEEmulation: isEnabled("emulate_gce", true) && isEnabled("devshell", true),
		EnableDockerAuth:   isEnabled("docker_auth", true),
		EnableFirebaseAuth: isEnabled("firebase_auth", false),
		KnownGerritHosts:   args.KnownPublicGerritHosts,
	}

	// Additional scopes for APIs not covered by 'cloud-platform' scope.
	if user.EnableGitAuth {
		user.Options.Scopes = append(authOpts.Scopes, "https://www.googleapis.com/auth/gerritcodereview")
	}
	if user.EnableFirebaseAuth {
		user.Options.Scopes = append(authOpts.Scopes, "https://www.googleapis.com/auth/firebase")
	}

	authDir, err := filepath.Abs("auth-configs")
	if err != nil {
		return nil, errors.Annotate(err, "failed to get absolute path").Err()
	}
	if err := os.Mkdir(authDir, 0700); err != nil {
		return nil, errors.Annotate(err, "failed to mkdir %s", authDir).Err()
	}
	if err := user.Launch(ctx, authDir); err != nil {
		return nil, errors.Annotate(err, "failed to start user auth context").Err()
	}

	// Log the actual service account email and enabled options.
	user.Report(ctx)
	return user, nil
}
