// Copyright 2018 The LUCI Authors.
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

package lucicfg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	"github.com/golang/protobuf/proto"
	"go.starlark.net/starlark"

	config "go.chromium.org/luci/common/api/luci_config/config/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/starlark/builtins"
	"go.chromium.org/luci/starlark/starlarkproto"
)

// Alias some ridiculously long type names that we round-trip in the public API.
type (
	ValidationRequest = config.LuciConfigValidateConfigRequestMessage
	ValidationMessage = config.ComponentsConfigEndpointValidationMessage
)

// ConfigSet is an in-memory representation of a config set.
//
// Keys are slash-separated filenames, values are corresponding file bodies.
type ConfigSet map[string][]byte

// ValidationResult is what we get after validating a config set.
type ValidationResult struct {
	Failed   bool                 `json:"failed"`   // true if the config is bad
	Messages []*ValidationMessage `json:"messages"` // errors, warning, infos, etc.
}

// ConfigSetValidator is primarily implemented through config.Service, but can
// also be mocked in tests.
type ConfigSetValidator interface {
	// Validate sends the validation request to the service.
	Validate(ctx context.Context, req *ValidationRequest) (*ValidationResult, error)
}

type remoteValidator struct {
	svc *config.Service
}

func (r remoteValidator) Validate(ctx context.Context, req *ValidationRequest) (*ValidationResult, error) {
	resp, err := r.svc.ValidateConfig(req).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return &ValidationResult{Messages: resp.Messages}, nil
}

// RemoteValidator returns ConfigSetValidator that makes RPCs to LUCI Config.
func RemoteValidator(svc *config.Service) ConfigSetValidator {
	return remoteValidator{svc}
}

// ReadConfigSet reads all regular files in the given directory (recursively)
// and returns them as a ConfigSet.
func ReadConfigSet(dir string) (ConfigSet, error) {
	configs := ConfigSet{}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || !info.Mode().IsRegular() {
			return err
		}
		content, err := ioutil.ReadFile(p)
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		configs[filepath.ToSlash(relPath)] = content
		return nil
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to read config files").Err()
	}
	return configs, nil
}

// Compare compares files on disk to what's in the config set.
//
// Returns names of files that are different ('changed') and same ('unchanged').
//
// Files on disk that are not in the config set are totally ignored. Files that
// cannot be read are considered outdated.
func (cs ConfigSet) Compare(dir string) (changed, unchanged []string) {
	for _, name := range cs.Files() {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if bytes.Equal(fileDigest(path), blobDigest(cs[name])) {
			unchanged = append(unchanged, name)
		} else {
			changed = append(changed, name)
		}
	}
	return
}

// Write updates files on disk to match the config set.
//
// Returns a list of updated files and a list of files that are already
// up-to-date, same as Compare.
//
// Creates missing directories. Not atomic. All files have mode 0666.
func (cs ConfigSet) Write(dir string) (changed, unchanged []string, err error) {
	// First pass: populate 'changed' and 'unchanged', so we have a valid result
	// even when failing midway through writes.
	changed, unchanged = cs.Compare(dir)

	// Second pass: update changed files.
	for _, name := range changed {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(path), 0777); err != nil {
			return
		}
		if err = ioutil.WriteFile(path, cs[name], 0666); err != nil {
			return
		}
	}

	return
}

// Digests returns a map "config name -> hex SHA256 of its body".
func (cs ConfigSet) Digests() map[string]string {
	out := make(map[string]string, len(cs))
	for file, body := range cs {
		out[file] = hex.EncodeToString(blobDigest(body))
	}
	return out
}

// Files returns a sorted list of file names in the config set.
func (cs ConfigSet) Files() []string {
	f := make([]string, 0, len(cs))
	for k := range cs {
		f = append(f, k)
	}
	sort.Strings(f)
	return f
}

// DebugDump writes the config files to stdout in a format useful for debugging.
func (cs ConfigSet) DebugDump() {
	for _, f := range cs.Files() {
		fmt.Println("--------------------------------------------------")
		fmt.Println(f)
		fmt.Println("--------------------------------------------------")
		fmt.Print(string(cs[f]))
		fmt.Println("--------------------------------------------------")
	}
}

// DiscardChangesToUntracked replaces bodies of the files that are in the config
// set, but not in the `tracked` set (per TrackedSet semantics) with what's on
// disk in the given `dir`.
//
// This allows to construct partially generated config set: some configs (the
// ones in the tracked set) are generated, others are loaded from disk.
//
// If `dir` is "-" (which indicates that the config set is going to be dumped
// to stdout rather then to disk), just removes untracked files from the config
// set.
func (cs ConfigSet) DiscardChangesToUntracked(ctx context.Context, tracked []string, dir string) error {
	isTracked := TrackedSet(tracked)

	for _, path := range cs.Files() {
		yes, err := isTracked(path)
		if err != nil {
			return err
		}
		if yes {
			continue
		}

		logging.Warningf(ctx, "Discarding changes to %s, not in the tracked set", path)

		if dir == "-" {
			// When using stdout as destination, there's nowhere to read existing
			// files from.
			delete(cs, path)
			continue
		}

		switch body, err := ioutil.ReadFile(filepath.Join(dir, filepath.FromSlash(path))); {
		case err == nil:
			cs[path] = body
		case os.IsNotExist(err):
			delete(cs, path)
		case err != nil:
			return errors.Annotate(err, "when discarding changes to %s", path).Err()
		}
	}

	return nil
}

// Validate sends the config set for validation to LUCI Config service.
//
// 'name' is a name of this config set from LUCI Config point of view, e.g.
// "projects/<something>" or "services/<something>". It tells LUCI Config how
// to validate files in the set.
//
// Returns an error only if the validation call itself failed (e.g. LUCI Config
// was unreachable). Otherwise returns ValidationResult with a list of
// validation message (errors, warnings, etc). The list of messages may be empty
// if the config set is 100% valid.
func (cs ConfigSet) Validate(ctx context.Context, name string, val ConfigSetValidator) (*ValidationResult, error) {
	req := &ValidationRequest{
		ConfigSet: name,
		Files:     make([]*config.LuciConfigValidateConfigRequestMessageFile, len(cs)),
	}

	logging.Infof(ctx, "Sending for validation to LUCI Config...")
	for idx, f := range cs.Files() {
		logging.Debugf(ctx, "  %s (%d bytes)", f, len(cs[f]))
		req.Files[idx] = &config.LuciConfigValidateConfigRequestMessageFile{
			Path:    f,
			Content: base64.StdEncoding.EncodeToString(cs[f]),
		}
	}

	res, err := val.Validate(ctx, req)
	if err != nil {
		return nil, errors.Annotate(err, "error validating configs").Err()
	}
	return res, nil
}

// Log all messages in the result to the logger at an appropriate logging level.
func (vr *ValidationResult) Log(ctx context.Context) {
	for _, msg := range vr.Messages {
		lvl := logging.Info
		switch msg.Severity {
		case "WARNING":
			lvl = logging.Warning
		case "ERROR", "CRITICAL":
			lvl = logging.Error
		}
		logging.Logf(ctx, lvl, "%s: %s", msg.Path, msg.Text)
	}
}

// OverallError is nil if the validation succeeded or non-nil if failed.
//
// Beware: mutates Failed field accordingly.
func (vr *ValidationResult) OverallError(failOnWarnings bool) error {
	errs, warns := 0, 0
	for _, msg := range vr.Messages {
		switch msg.Severity {
		case "WARNING":
			warns++
		case "ERROR", "CRITICAL":
			errs++
		}
	}

	if errs > 0 {
		vr.Failed = true
		return errors.Reason("some files were invalid").Err()
	}
	if warns > 0 && failOnWarnings {
		vr.Failed = true
		return errors.Reason("some files had validation warnings and -fail-on-warnings is set").Err()
	}

	vr.Failed = false
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// Misc helpers used above.

// fileDigest returns SHA256 digest of a file on disk or nil if it's not there
// or can't be read.
func fileDigest(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil
	}
	return h.Sum(nil)
}

// blobDigest returns SHA256 digest of a byte buffer.
func blobDigest(blob []byte) []byte {
	d := sha256.Sum256(blob)
	return d[:]
}

////////////////////////////////////////////////////////////////////////////////
// Constructing config sets from Starlark (tested through starlark_test.go).

// configSetValue is a map-like starlark.Value that has file names as keys and
// strings or protobuf messages as values.
//
// At the end of the execution all protos are serialized to strings too, using
// textpb encoding.
type configSetValue struct {
	starlark.Dict
}

func newConfigSetValue() *configSetValue {
	return &configSetValue{}
}

func (cs *configSetValue) Type() string { return "config_set" }

func (cs *configSetValue) SetKey(k, v starlark.Value) error {
	if _, ok := k.(starlark.String); !ok {
		return fmt.Errorf("config set key should be a string, not %s", k.Type())
	}

	_, str := v.(starlark.String)
	_, msg := v.(*starlarkproto.Message)
	if !str && !msg {
		return fmt.Errorf("config set value should be either a string or a proto message, not %s", v.Type())
	}

	return cs.Dict.SetKey(k, v)
}

// renderWithTextProto returns a config set with all protos serialized to text
// proto format (with added header).
//
// Configs supplied as strings are serialized using UTF-8 encoding.
func (cs *configSetValue) renderWithTextProto(header string) (ConfigSet, error) {
	out := make(ConfigSet, cs.Len())

	for _, kv := range cs.Items() {
		k, v := kv[0].(starlark.String), kv[1]

		text := ""
		if s, ok := v.(starlark.String); ok {
			text = s.GoString()
		} else {
			msg, err := v.(*starlarkproto.Message).ToProto()
			if err != nil {
				return nil, err
			}
			text = header + proto.MarshalTextString(msg)
		}

		out[k.GoString()] = []byte(text)
	}

	return out, nil
}

// generators is a list of registered generator callbacks.
//
// It lives in State. Generators are executed sequentially after all Starlark
// code is loaded. They examine the state and generate configs based on it.
type generators struct {
	gen        []starlark.Callable
	runningNow bool // true while inside 'call'
}

// add registers a new generator callback.
func (g *generators) add(cb starlark.Callable) error {
	if g.runningNow {
		return fmt.Errorf("can't add a generator while already running them")
	}
	g.gen = append(g.gen, cb)
	return nil
}

// call calls all registered callbacks sequentially, collecting all errors.
func (g *generators) call(th *starlark.Thread, ctx *genCtx) (errs errors.MultiError) {
	if g.runningNow {
		return errors.MultiError{
			fmt.Errorf("can't call generators while they are already running"),
		}
	}
	g.runningNow = true
	defer func() { g.runningNow = false }()

	fc := builtins.GetFailureCollector(th)

	for _, cb := range g.gen {
		if fc != nil {
			fc.Clear()
		}
		if _, err := starlark.Call(th, cb, starlark.Tuple{ctx}, nil); err != nil {
			if fc != nil && fc.LatestFailure() != nil {
				// Prefer this error, it has custom stack trace.
				errs = append(errs, fc.LatestFailure())
			} else {
				errs = append(errs, err)
			}
		}
	}
	return
}

func init() {
	// new_config_set() makes a new empty config set, useful in tests.
	declNative("new_config_set", func(call nativeCall) (starlark.Value, error) {
		if err := call.unpack(0); err != nil {
			return nil, err
		}
		return newConfigSetValue(), nil
	})

	// add_generator(cb) registers a callback that is called at the end of the
	// execution to generate or mutate produced configs.
	declNative("add_generator", func(call nativeCall) (starlark.Value, error) {
		var cb starlark.Callable
		if err := call.unpack(1, &cb); err != nil {
			return nil, err
		}
		return starlark.None, call.State.generators.add(cb)
	})

	// call_generators(ctx) calls all registered generators, useful in tests.
	declNative("call_generators", func(call nativeCall) (starlark.Value, error) {
		var ctx *genCtx
		if err := call.unpack(1, &ctx); err != nil {
			return nil, err
		}
		switch errs := call.State.generators.call(call.Thread, ctx); {
		case len(errs) == 0:
			return starlark.None, nil
		case len(errs) == 1:
			return starlark.None, errs[0]
		default:
			return starlark.None, errs
		}
	})
}
