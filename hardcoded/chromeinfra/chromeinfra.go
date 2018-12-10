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
	// CIPDServiceURL is URL of a CIPD backend to connect to by default.
	CIPDServiceURL = "https://chrome-infra-packages.appspot.com"

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

	// MachineDatabaseDevURL is the URL of the Machine Database dev instance.
	MachineDatabaseDevHost = "machine-db-dev.appspot.com"

	// ConfigServiceHost is the default host of LUCI config service.
	ConfigServiceHost = "luci-config.appspot.com"
)

// DefaultAuthOptions returns auth.Options struct prefilled with chrome-infra
// defaults.
func DefaultAuthOptions() auth.Options {
	// Note that ClientSecret is not really a secret since it's hardcoded into
	// the source code (and binaries). It's totally fine, as long as it's callback
	// URI is configured to be 'localhost'. If someone decides to reuse such
	// ClientSecret they have to run something on user's local machine anyway
	// to get the refresh_token.
	return auth.Options{
		ClientID:     "446450136466-2hr92jrq8e6i4tnsa56b52vacp7t3936.apps.googleusercontent.com",
		ClientSecret: "uBfbay2KCy9t4QveJ-dOqHtp",
		SecretsDir:   SecretsDir(),
	}
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
