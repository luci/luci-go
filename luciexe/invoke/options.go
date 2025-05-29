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

// Package invoke implements the process of invoking a 'luciexe' compatible
// subprocess, but without setting up any of the 'host' requirements (like
// a Logdog Butler or LUCI Auth).
//
// See go.chromium.org/luci/luciexe for details on the protocol.
package invoke

import (
	"context"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/protobuf/ptypes"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/lucictx"

	"go.chromium.org/luci/luciexe"
)

// Options represents settings to use when Start'ing a luciexe.
//
// All values here have defaults, so Start can accept `nil`.
type Options struct {
	// This should be in Build.Step.Name format, i.e. "parent|parent|leaf".
	//
	// Namespace is used as:
	//   * The Step name of the enclosing Build Step.
	//   * Generating the new LOGDOG_NAMESPACE in the environment for the
	//     subprocess. Non-StreamName characters are replaced with '_',
	//     and if the first character of a segment is not a character, "s_" is
	//     prpended. i.e. if the current LOGDOG_NAMESPACE is "u", and this
	//     namespace is "x|!!cool!!|z", then LOGDOG_NAMESPACE for the
	//     subprocess will be "u/x/s___cool__/z".
	//     * The LUCI Executable host process will assert that all logdog stream
	//       names mentioned in the subprocess's Build are relative to this.
	//   * The basis for the stdout/stderr Logdog streams for the subprocess.
	//   * The LUCI Executable host process will adjust the subprocess's step
	//     names to be relative to this. i.e. if the subprocess emits a step "a|b"
	//     and the Namespace is "x|y|z", then the step will show up as "x|y|z|a|b"
	//     on the overall Build.
	//
	// TODO(iannucci): Logdog stream names are restricted because they show up in
	// URLs on the logdog service. If the logdog service instead uses URL encoding
	// of the stream name, then StreamName could be expanded to allow all
	// characters except for "/". At some later point we could potentially change
	// the separator from "/" to "|" to match Buildbucket semantics.
	//
	// TODO(iannucci): Find a way to have logdog stream name namespacing be more
	// transparent (i.e. without needing explicit cooperation from the logdog
	// client library via LOGDOG_NAMESPACE envvar).
	//
	// Default: The Namespace is empty. This is ONLY useful when writing
	// a transparent wrapper for a luciexe which doesn't, itself, implement the
	// luciexe protocol. If Namespace is empty, then Subprocess.Step will be
	// `nil`.
	Namespace string

	// The base dir where all sub-directories (i.e. workdir, tempdir, etc.)
	// will be created under (i.e. the `cwd` for invoked luciexe will be
	// `$BaseDir/w`). If specified, this must exist and be a directory.
	//
	// If empty, a random directory under os.TempDir will be used.
	BaseDir string

	// Absolute path to the cache base directory. This must exist and be
	// a directory.
	//
	// Default: If LUCI_CONFIG['lucictx']['cache_dir'] is set, it will be passed
	// through unchanged. Otherwise a new empty directory is allocated which will
	// be destroyed on the subprocess's completion.
	CacheDir string

	// If set, the subprocess's final Build message will be collected and returned
	// from Wait.
	//
	// If CollectOutput is specified, but CollectOutputPath is not, a temporary
	// path will be seleceted and destroyed on the subprocess's completion.
	CollectOutput bool

	// If set, will be used as the path to output the subprocess's final Build
	// state. Must end with one of {'.pb', '.json', '.textpb'}. This must be
	// a path to a non-existent file (i.e. parent must be a directory).
	//
	// If CollectOutputPath is specified, but CollectOutput is not, the subprocess
	// will be instructed to dump the result to this path, but we won't attempt to
	// parse or validate it in any way, and Wait will return a nil `output`.
	CollectOutputPath string

	// A replacement environment for the Options.
	//
	// If this is not specified, the current process's environment is inherited.
	//
	// The following environment variables will be ignored from this `Env`:
	//   * LOGDOG_NAMESPACE
	//   * LUCI_CONTEXT
	Env environ.Env
}

// launchOptions is a 'digested' form of Options, used for starting
// a subprocess.
type launchOptions struct {
	// lctx is the exported LUCI_CONTEXT object must be Close()'d by the caller of
	// Options.rationalize. It's already been set in `env`.
	lctx lucictx.Exported

	// step is the constructed Step object for the user of the invoke library. It
	// may be nil if Namespace was omitted.
	step *bbpb.Step

	// workDir is the working directory for the invoked luciexe.
	workDir string

	// args are the CLI arguments to the luciexe.
	args []string

	// These are the open streams ready to attach to the subprocess.
	stdout io.WriteCloser
	stderr io.WriteCloser

	// collectPath, if set, is the build file to read after the completion of the
	// subprocess.
	collectPath string

	// env is an environment suitable to run the luciexe in.
	env environ.Env
}

func (o *Options) prepNamespace(ctx context.Context, lo *launchOptions) error {
	if o.Namespace == "" {
		return nil
	}

	var bits []string
	bits = append(bits, strings.Split(o.Namespace, "|")...)
	relNS, _ := types.MakeStreamName("s_", bits...)

	// The $LOGDOG_NAMESPACE envvar is currently cumulative, even though the
	// Log.Url field is relative.
	fullNS := string(relNS)
	if curNS := lo.env.Get(luciexe.LogdogNamespaceEnv); curNS != "" {
		fullNS = strings.Join([]string{curNS, fullNS}, "/")
	}
	lo.env.Set(luciexe.LogdogNamespaceEnv, fullNS)

	startTime, err := ptypes.TimestampProto(clock.Now(ctx))
	if err != nil {
		return errors.Fmt("invalid StartTime: %w", err)
	}
	lo.step = &bbpb.Step{
		Name:      o.Namespace,
		StartTime: startTime,
		Status:    bbpb.Status_STARTED,
		Logs: []*bbpb.Log{
			{
				Name: "stdout",
				Url:  string(relNS.Concat("stdout")),
			},
			{
				Name: "stderr",
				Url:  string(relNS.Concat("stderr")),
			},
		},
		MergeBuild: &bbpb.Step_MergeBuild{
			FromLogdogStream: string(relNS.Concat(luciexe.BuildProtoStreamSuffix)),
		},
	}
	return nil
}

func (o *Options) prepCacheDir(ctx context.Context, cdir string, lo *launchOptions) (newCtx context.Context, err error) {
	newCtx = ctx // so we don't trip up our caller
	newDir := o.CacheDir

	if newDir == "" {
		if curval := lucictx.GetLUCIExe(ctx); curval != nil && curval.CacheDir == "" {
			err = errors.New(
				`$LUCI_CONTEXT["luciexe"] is set, but "cache_dir" is empty`)
			return
		}
		newDir = cdir
	} else {
		if newDir, err = filepath.Abs(newDir); err != nil {
			return nil, errors.Fmt("resolving CacheDir: %w", err)
		}
		if err = checkDirExists(newDir); err != nil {
			return nil, errors.Fmt("checking CacheDir: %w", err)
		}
	}

	newCtx = lucictx.SetLUCIExe(ctx, &lucictx.LUCIExe{CacheDir: newDir})
	lo.lctx, err = lucictx.Export(newCtx)
	lo.lctx.SetInEnviron(lo.env)
	return
}

func (o *Options) prepCollection(outDir string, lo *launchOptions) error {
	if !o.CollectOutput && o.CollectOutputPath == "" {
		return nil
	}

	collect := o.CollectOutputPath
	if collect == "" {
		collect = filepath.Join(outDir, "out"+luciexe.BuildFileCodecBinary.FileExtension())
	} else {
		if err := checkDirExists(filepath.Dir(collect)); err != nil {
			return errors.Fmt("checking CollectOutputPath's parent: %w", err)
		}

		if _, err := os.Stat(collect); !os.IsNotExist(err) {
			return errors.Fmt("CollectOutputPath points to an existing file: %q", collect)
		}
	}

	if _, err := luciexe.BuildFileCodecForPath(collect); err != nil {
		return errors.Fmt("CollectOutputPath: %w", err)
	}

	lo.args = []string{luciexe.OutputCLIArg, collect}
	lo.collectPath = collect
	return nil
}

type dirs struct {
	// May or may not be used; for simplicity we always create them under dirs,
	// but rationalize may override them if the user specified a specific location
	// for them in Options.
	cacheDir         string
	collectOutputDir string

	// always used
	tempDir string
	workDir string
}

func (o *Options) mkdirs() (ret dirs, err error) {
	base := o.BaseDir
	if base == "" {
		if base, err = ioutil.TempDir("", ""); err != nil {
			return
		}
	}
	if err = checkDirExists(base); err != nil {
		return ret, errors.Fmt("checking BaseDir: %w", err)
	}

	// maybeMkdir attempts to make the dir named `dirname` under `base`,
	// annotating the error with `friendlyName` as long as `err` is nil.
	//
	// Updates `err` with the result of the mkdir call as a side effect.
	maybeMkdir := func(out *string, dirname, friendlyName string) {
		if err == nil {
			*out = filepath.Join(base, dirname)
			err = errors.WrapIf(os.Mkdir(*out, 0777), "preparing %q", friendlyName)
		}
	}

	maybeMkdir(&ret.cacheDir, "c", "cache-dir")
	maybeMkdir(&ret.collectOutputDir, "o", "output-dir")
	maybeMkdir(&ret.tempDir, "t", "temp-dir")
	maybeMkdir(&ret.workDir, "w", "work-dir")

	return
}

func (lo *launchOptions) prepStdio(ctx context.Context) error {
	// NOTE: bootstrapping doesn't do any 'write' actions to the logdog state and
	// is a fancy way of reading a couple of envvars and building a struct (i.e.
	// this is quite cheap).
	bs, err := bootstrap.GetFromEnv(lo.env) // picks up Namespace
	switch err {
	case nil:
	case bootstrap.ErrNotBootstrapped:
		return errors.New("Logdog Butler environment required")
	default:
		return errors.Fmt("bootstrapping logdog client: %w", err)
	}

	openStream := func(name string) (ret io.WriteCloser, err error) {
		ret, err = bs.Client.NewStream(
			ctx, types.StreamName(name), streamclient.ForProcess())
		err = errors.WrapIf(err, "opening %q", name)
		return
	}

	if lo.stdout, err = openStream("stdout"); err != nil {
		return err
	}
	if lo.stderr, err = openStream("stderr"); err != nil {
		lo.stdout.Close()
		return err
	}
	return nil
}

// rationalize converts from a set of requested Options to a usable
// launchOptions object.
func (o *Options) rationalize(ctx context.Context) (ret launchOptions, newCtx context.Context, err error) {
	if o == nil {
		o = &Options{}
	}
	if o.Env.Len() != 0 {
		ret.env = o.Env.Clone()
	} else {
		ret.env = environ.System()
	}

	var d dirs
	if d, err = o.mkdirs(); err != nil {
		return
	}
	ret.workDir = d.workDir
	for _, key := range luciexe.TempDirEnvVars {
		ret.env.Set(key, d.tempDir)
	}

	if newCtx, err = o.prepCacheDir(ctx, d.cacheDir, &ret); err != nil {
		err = errors.Fmt("preparing cachedir: %w", err)
		return
	}

	if err = o.prepNamespace(newCtx, &ret); err != nil {
		err = errors.Fmt("preparing namespace: %w", err)
		return
	}

	if err = o.prepCollection(d.collectOutputDir, &ret); err != nil {
		err = errors.Fmt("preparing collection: %w", err)
		return
	}

	if err = ret.prepStdio(newCtx); err != nil {
		err = errors.Fmt("preparing outputs: %w", err)
		return
	}

	return
}

// checkDirExists returns error if the given path is not an
// existing directory.
func checkDirExists(path string) error {
	fInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.Fmt("dir does not exist: %q", path)
		}
		return errors.Fmt("statting path: %q: %w", path, err)
	}
	if !fInfo.IsDir() {
		return errors.Fmt("path is not a directory: %q", path)
	}
	return nil
}
