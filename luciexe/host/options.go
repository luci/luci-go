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

package host

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/authctx"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	ldOutput "go.chromium.org/luci/logdog/client/butler/output"
)

// Options is an optional struct which allows you to control how Run operates.
type Options struct {
	// Where the butler will sink its data to.
	//
	// This is typically one of the implementations in
	// go.chromium.org/luci/logdog/client/butler/output.
	//
	// If nil, will use the 'null' logdog Output.
	LogdogOutput ldOutput.Output

	// ExeAuth describes the LUCI Auth environment to run the user code within.
	//
	// `Run` will manage the lifecycle of ExeAuth entirely.
	//
	// If nil, defaults to `DefaultExeAuth("luciexe", nil)`.
	//
	// It's recommended to use DefaultExeAuth() explicitly with reasonable values
	// for `id` and `knownGerritHosts`.
	ExeAuth *authctx.Context

	// The BaseDir becomes the root of this hosted luciexe session; All
	// directories (workdirs, tempdirs, etc.) are derived relative to this.
	//
	// If not provided, Run will pick a random directory under os.TempDir as the
	// value for BaseDir.
	//
	// The BaseDir (provided or picked) will be managed by Run; Prior to
	// execution, Run will ensure that the directory is empty.
	BaseDir string

	authDir    string
	lucictxDir string
}

// The function below is in `var name = func` form so that it shows up in godoc.

// DefaultExeAuth returns a copy of the default value for Options.ExeAuth.
var DefaultExeAuth = func(id string, knownGerritHosts []string) *authctx.Context {
	return &authctx.Context{
		ID: id,
		Options: chromeinfra.SetDefaultAuthOptions(auth.Options{
			Scopes: []string{
				"https://www.googleapis.com/auth/cloud-platform",
				"https://www.googleapis.com/auth/userinfo.email",
				"https://www.googleapis.com/auth/gerritcodereview",
				"https://www.googleapis.com/auth/firebase",
			},
		}),
		EnableGitAuth:      true,
		EnableGCEEmulation: true,
		EnableDockerAuth:   true,
		EnableFirebaseAuth: true,
		KnownGerritHosts:   knownGerritHosts,
	}
}

func (o *Options) initialize() (err error) {
	if o.BaseDir == "" {
		o.BaseDir, err = ioutil.TempDir("", "luciexe-host-")
		if err != nil {
			return errors.Annotate(err, "generating options.BaseDir").Err()
		}
	}

	pathsToMake := []struct {
		path string  // unique directory name under BaseDir
		name string  // human readable name of this dir (i.e. it's purpose)
		dest *string // output destination in Options struct
	}{
		{"a", "auth", &o.authDir},
		{"l", "luci context", &o.lucictxDir},
	}

	merr := errors.NewLazyMultiError(len(pathsToMake))
	for i, paths := range pathsToMake {
		*paths.dest = filepath.Join(o.BaseDir, paths.path)
		merr.Assign(i, errors.Annotate(os.Mkdir(*paths.dest, 0777), "making %q dir", paths.name).Err())
	}
	if err := merr.Get(); err != nil {
		return err
	}

	if o.ExeAuth == nil {
		o.ExeAuth = DefaultExeAuth("luciexe", nil)
	}

	return nil
}
