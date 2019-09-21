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
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/authctx"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	ldOutput "go.chromium.org/luci/logdog/client/butler/output"
	"go.chromium.org/luci/logdog/client/butler/output/null"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
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
	// execution, Run will ensure that the directory is empty by removing its
	// contents.
	BaseDir string

	// The base Build message to use as the template for all merged Build
	// messages.
	BaseBuild *bbpb.Build

	// If LeakBaseDir is true, Run will not try to remove BaseDir at the end if
	// it's execution.
	//
	// If BaseDir is not provided, this must be false.
	LeakBaseDir bool

	// The viewer URL for this hosted execution (if any). This will be used to
	// apply the "logdog.viewer_url" tag to all logdog streams (the tag which is
	// used to implement the "Back to build" link in Milo).
	ViewerURL string

	authDir          string
	lucictxDir       string
	streamServerPath string

	logdogTags streamproto.TagMap
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

type pathToMake struct {
	path string  // unique directory name under BaseDir
	name string  // human readable name of this dir (i.e. it's purpose)
	dest *string // output destination in Options struct
}

func (p pathToMake) create(base string) error {
	*p.dest = filepath.Join(base, p.path)
	return errors.Annotate(os.Mkdir(*p.dest, 0777), "making %q dir", p.name).Err()
}

func (o *Options) initialize() (err error) {
	if o.BaseDir == "" {
		if o.LeakBaseDir {
			return errors.New("BaseDir was provided but LeakBaseDir == true")
		}
		o.BaseDir, err = ioutil.TempDir("", "luciexe-host-")
		if err != nil {
			return errors.Annotate(err, "Cannot create BaseDir").Err()
		}
	} else {
		if err := os.RemoveAll(o.BaseDir); err != nil && !os.IsNotExist(err) {
			return errors.Annotate(err, "clearing options.BaseDir").Err()
		}
		if err := os.Mkdir(o.BaseDir, 0777); err != nil {
			return errors.Annotate(err, "creating options.BaseDir").Err()
		}
	}

	if o.BaseBuild == nil {
		o.BaseBuild = &bbpb.Build{}
	}

	pathsToMake := []pathToMake{
		{"a", "auth", &o.authDir},
		{"l", "luci context", &o.lucictxDir},
	}

	if runtime.GOOS != "windows" {
		pathsToMake = append(
			pathsToMake, pathToMake{"ld", "logdog socket", &o.streamServerPath})
	} else {
		o.streamServerPath = "luciexe/host"
	}

	merr := errors.NewLazyMultiError(len(pathsToMake))
	for i, paths := range pathsToMake {
		merr.Assign(i, paths.create(o.BaseDir))
	}
	if err := merr.Get(); err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		o.streamServerPath = filepath.Join(o.streamServerPath, "sock")
	}

	if o.LogdogOutput == nil {
		o.LogdogOutput = &null.Output{}
	}

	if o.ExeAuth == nil {
		o.ExeAuth = DefaultExeAuth("luciexe", nil)
	}

	if o.ViewerURL != "" {
		o.logdogTags = streamproto.TagMap{"logdog.viewer_url": o.ViewerURL}
	}

	return nil
}

func (o *Options) cleanup(ctx context.Context) {
	if !o.LeakBaseDir {
		if err := os.RemoveAll(o.BaseDir); err != nil {
			logging.WithError(err).Errorf(ctx, "removing BaseDir")
		}
	}
}
