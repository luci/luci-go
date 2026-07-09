// Copyright 2026 The LUCI Authors.
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

package base

import (
	"context"
	"flag"
	"net/http"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

type AuthFlags struct {
	Flags       authcli.Flags
	DefaultOpts auth.Options
	ParsedOpts  *auth.Options
}

func NewAuthFlags() *AuthFlags {
	return &AuthFlags{
		DefaultOpts: chromeinfra.DefaultAuthOptions(),
	}
}

func (af *AuthFlags) Register(f *flag.FlagSet) {
	af.Flags.Register(f, af.DefaultOpts)
}

func (af *AuthFlags) Parse() error {
	opts, err := af.Flags.Options()
	if err != nil {
		return err
	}
	af.ParsedOpts = &opts
	return nil
}

func (af *AuthFlags) NewHTTPClient(ctx context.Context) (*http.Client, error) {
	if af.ParsedOpts == nil {
		return nil, errors.New("AuthFlags.Parse() must be called")
	}
	return auth.NewAuthenticator(ctx, auth.OptionalLogin, *af.ParsedOpts).Client()
}
