// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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

	"github.com/luci/luci-go/common/auth"
	homedir "github.com/mitchellh/go-homedir"
)

// TODO(vadimsh): Move the rest of hardcoded stuff here:
//  * tsmon config file: "/etc/chrome-infra/ts-mon.json"
//  * tsmon secrets dir: same as SecretsDir below.
//  * tsmon network detection regexp: `^([\w-]*?-[acm]|master)(\d+)a?$`
//  * cipd service URL: "https://chrome-infra-packages.appspot.com"

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
