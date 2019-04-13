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

package lucicfg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/golang/protobuf/proto"
	"go.starlark.net/starlark"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/starlark/starlarkproto"
)

// Output is an in-memory representation of all generated output files.
//
// Output may span zero or more config sets, each defined by its root directory.
// Config sets may intersect (though this is rare).
type Output struct {
	// Data is all output files.
	//
	// Keys are slash-separated filenames, values are corresponding file bodies.
	Data map[string][]byte

	// Roots is mapping "config set name => its root".
	//
	// Roots are given as slash-separated paths relative to the output root, e.g.
	// '.' matches ALL output files.
	Roots map[string]string
}

// ConfigSets partitions this output into 0 or more config sets based on Roots.
func (o Output) ConfigSets() []ConfigSet {
	names := make([]string, 0, len(o.Roots))
	for name := range o.Roots {
		names = append(names, name)
	}
	sort.Strings(names) // order is important for logs

	cs := make([]ConfigSet, len(names))
	for i, nm := range names {
		root := o.Roots[nm]

		// Normalize in preparation for prefix matching.
		root = path.Clean(root)
		if root == "." {
			root = "" // match EVERYTHING
		} else {
			root = root + "/" // match only what's under 'root/...'
		}

		files := map[string][]byte{}
		for f, body := range o.Data {
			f = path.Clean(f)
			if strings.HasPrefix(f, root) {
				files[f[len(root):]] = body
			}
		}

		cs[i] = ConfigSet{Name: nm, Data: files}
	}

	return cs
}

// Compare compares files on disk to what's in the output.
//
// Returns names of files that are different ('changed') and same ('unchanged').
//
// Files on disk that are not in the output set are totally ignored. Files that
// cannot be read are considered outdated.
func (o Output) Compare(dir string) (changed, unchanged []string) {
	for _, name := range o.Files() {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if bytes.Equal(fileDigest(path), blobDigest(o.Data[name])) {
			unchanged = append(unchanged, name)
		} else {
			changed = append(changed, name)
		}
	}
	return
}

// Write updates files on disk to match the output.
//
// Returns a list of updated files and a list of files that are already
// up-to-date, same as Compare.
//
// Creates missing directories. Not atomic. All files have mode 0666.
func (o Output) Write(dir string) (changed, unchanged []string, err error) {
	// First pass: populate 'changed' and 'unchanged', so we have a valid result
	// even when failing midway through writes.
	changed, unchanged = o.Compare(dir)

	// Second pass: update changed files.
	for _, name := range changed {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(path), 0777); err != nil {
			return
		}
		if err = ioutil.WriteFile(path, o.Data[name], 0666); err != nil {
			return
		}
	}

	return
}

// Digests returns a map "file name -> hex SHA256 of its body".
func (o Output) Digests() map[string]string {
	out := make(map[string]string, len(o.Data))
	for file, body := range o.Data {
		out[file] = hex.EncodeToString(blobDigest(body))
	}
	return out
}

// Files returns a sorted list of file names in the output.
func (o Output) Files() []string {
	f := make([]string, 0, len(o.Data))
	for k := range o.Data {
		f = append(f, k)
	}
	sort.Strings(f)
	return f
}

// DebugDump writes the output to stdout in a format useful for debugging.
func (o Output) DebugDump() {
	for _, f := range o.Files() {
		fmt.Println("--------------------------------------------------")
		fmt.Println(f)
		fmt.Println("--------------------------------------------------")
		fmt.Print(string(o.Data[f]))
		fmt.Println("--------------------------------------------------")
	}
}

// DiscardChangesToUntracked replaces bodies of the files that are in the output
// set, but not in the `tracked` set (per TrackedSet semantics) with what's on
// disk in the given `dir`.
//
// This allows to construct partially generated output: some configs (the ones
// in the tracked set) are generated, others are loaded from disk.
//
// If `dir` is "-" (which indicates that the output is going to be dumped to
// stdout rather then to disk), just removes untracked files from the output.
func (o Output) DiscardChangesToUntracked(ctx context.Context, tracked []string, dir string) error {
	isTracked := TrackedSet(tracked)

	for _, path := range o.Files() {
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
			delete(o.Data, path)
			continue
		}

		switch body, err := ioutil.ReadFile(filepath.Join(dir, filepath.FromSlash(path))); {
		case err == nil:
			o.Data[path] = body
		case os.IsNotExist(err):
			delete(o.Data, path)
		case err != nil:
			return errors.Annotate(err, "when discarding changes to %s", path).Err()
		}
	}

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
// Constructing Output from Starlark (tested through starlark_test.go).

// outputBuilder is a map-like starlark.Value that has file names as keys and
// strings or protobuf messages as values.
//
// At the end of the execution all protos are serialized to strings too, using
// textpb encoding, to get the final Output.
type outputBuilder struct {
	starlark.Dict
}

func newOutputBuilder() *outputBuilder {
	return &outputBuilder{}
}

func (o *outputBuilder) Type() string { return "output" }

func (o *outputBuilder) SetKey(k, v starlark.Value) error {
	if _, ok := k.(starlark.String); !ok {
		return fmt.Errorf("output set key should be a string, not %s", k.Type())
	}

	_, str := v.(starlark.String)
	_, msg := v.(*starlarkproto.Message)
	if !str && !msg {
		return fmt.Errorf("output set value should be either a string or a proto message, not %s", v.Type())
	}

	return o.Dict.SetKey(k, v)
}

// renderWithTextProto returns output files with all protos serialized to text
// proto format (with added header).
//
// Configs supplied as strings are serialized using UTF-8 encoding.
func (o *outputBuilder) renderWithTextProto(header string) (map[string][]byte, error) {
	out := make(map[string][]byte, o.Len())

	for _, kv := range o.Items() {
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

func init() {
	// new_output_builder() makes a new output builder, useful in tests.
	declNative("new_output_builder", func(call nativeCall) (starlark.Value, error) {
		if err := call.unpack(0); err != nil {
			return nil, err
		}
		return newOutputBuilder(), nil
	})
}
