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

// Package chromeinfra contains hardcoded values related to Chrome Infra.
//
// It is supposed to be imported only by leaf 'main' packages of various
// binaries. All non-main packages must not hardcode any environment related
// values and must accept them as parameters passed from 'main'.
package chromeinfra

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	homedir "github.com/mitchellh/go-homedir"
	"go.chromium.org/luci/auth"
)

// TODO(vadimsh): Move the rest of hardcoded stuff here:
//  * tsmon config file: "/etc/chrome-infra/ts-mon.json"
//  * tsmon secrets dir: same as SecretsDir below.
//  * tsmon network detection regexp: `^([\w-]*?-[acm]|master)(\d+)a?$`

const (
	// BuildbucketHost is the hostname of the Buildbucket service to connect to
	// by default.
	BuildbucketHost = "cr-buildbucket.appspot.com"

	// CIPDServiceURL is URL of a CIPD backend to connect to by default.
	CIPDServiceURL = "https://chrome-infra-packages.appspot.com"

	// ConfigServiceHost is the default host of LUCI config service.
	ConfigServiceHost = "config.luci.app"

	// LogDogHost is the default host of the production LogDog service in Chrome
	// Operations.
	LogDogHost = "logs.chromium.org"

	// LogDogHostAppSpot is the ".appspot.com" host equivalent of LogDogHost.
	LogDogHostAppSpot = "luci-logdog.appspot.com"

	// LogDogDevHost is the default host of the development LogDog service in
	// Chrome Operations.
	LogDogDevHost = "luci-logdog-dev.appspot.com"

	// MachineDatabaseHost is the URL of the Machine Database.
	MachineDatabaseHost = "machine-db.appspot.com"

	// MachineDatabaseDevHost is the URL of the Machine Database dev instance.
	MachineDatabaseDevHost = "machine-db-dev.appspot.com"

	// MiloUIHost is the hostname of the production Milo UI.
	MiloUIHost = "luci-milo.appspot.com"

	// MiloDevUIHost is the hostname of the development Milo UI.
	MiloDevUIHost = "luci-milo-dev.appspot.com"

	// UFSProdHost is the URL of the ufs service.
	UFSProdHost = "ufs.api.cr.dev"

	// UFSStagingHost is the URL of the staging ufs service.
	UFSStagingHost = "staging.ufs.api.cr.dev"

	// ResultDBHost is the hostname of the production ResultDB service.
	ResultDBHost = "results.api.cr.dev"

	// ResultDBStagingHost is the hostname of the staging ResultDB service.
	ResultDBStagingHost = "staging.results.api.cr.dev"

	// TestSpannerInstance is the name of the Spanner instance used for testing.
	TestSpannerInstance = "projects/chops-spanner-testing/instances/testing"

	// TokenServerHost is the default host to use in auth.Options.TokenServerHost.
	TokenServerHost = "luci-token-server.appspot.com"

	// TokenServerDevHost is the host of the LUCI Token Server dev instance.
	TokenServerDevHost = "luci-token-server-dev.appspot.com"

	// LoginSessionsHost is the host to use for CLI login UI by default.
	LoginSessionsHost = "ci.chromium.org"

	// RPCExplorerClientID is the default client ID used by RPC Explorer.
	RPCExplorerClientID = "446450136466-e77v49thuh5dculh78gumq3oncqe28m3.apps.googleusercontent.com"
)

// DefaultAuthOptions returns auth.Options struct prefilled with chrome-infra
// defaults.
func DefaultAuthOptions() auth.Options {
	return auth.Options{
		TokenServerHost:   TokenServerHost,
		SecretsDir:        SecretsDir(),
		LoginSessionsHost: LoginSessionsHost,
		ClientID:          "446450136466-mj75ourhccki9fffaq8bc1e50di315po.apps.googleusercontent.com",
		ClientSecret:      "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",
	}
}

// SetDefaultAuthOptions sets the chromeinfra defaults on `opts`, returning the
// updated Options.
func SetDefaultAuthOptions(opts auth.Options) auth.Options {
	dflts := DefaultAuthOptions()
	ret := opts
	ret.TokenServerHost = TokenServerHost
	ret.ClientID = dflts.ClientID
	ret.ClientSecret = dflts.ClientSecret
	ret.LoginSessionsHost = dflts.LoginSessionsHost
	ret.SecretsDir = dflts.SecretsDir
	return ret
}

var secrets struct {
	once sync.Once
	val  string
}

// SecretsDir returns an absolute path to a directory (in $HOME) to keep secret
// files in (e.g. OAuth refresh tokens) or an empty string if $HOME can't be
// determined (happens in some degenerate cases, it just disables auth token
// cache).
func SecretsDir() string {
	secrets.once.Do(func() {
		home, err := homedir.Dir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Can't resolve $HOME: %s", err)
		} else {
			secrets.val = filepath.Join(home, ".config", "chrome_infra", "auth")
		}
	})
	return secrets.val
}
