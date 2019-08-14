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
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/starlark/starlarkprotov2"
)

// Output is an in-memory representation of all generated output files.
//
// Output may span zero or more config sets, each defined by its root directory.
// Config sets may intersect (though this is rare).
type Output struct {
	// Data is all output files.
	//
	// Keys are slash-separated filenames, values are corresponding file bodies.
	Data map[string]Datum

	// Roots is mapping "config set name => its root".
	//
	// Roots are given as slash-separated paths relative to the output root, e.g.
	// '.' matches ALL output files.
	Roots map[string]string
}

// Datum represents one generated output file.
type Datum interface {
	// Bytes is a raw file body to put on disk.
	Bytes() []byte
	// SemanticallySame is true if 'other' represents the same datum.
	SemanticallySame(other []byte) bool
}

// BlobDatum is a Datum which is just a raw byte blob.
type BlobDatum []byte

// Bytes is a raw file body to put on disk.
func (b BlobDatum) Bytes() []byte { return b }

// SemanticallySame is true if 'other == b'.
func (b BlobDatum) SemanticallySame(other []byte) bool { return bytes.Equal(b, other) }

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
				files[f[len(root):]] = body.Bytes()
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
// Files on disk that are not in the output set are totally ignored. Missing
// files that are listed in the output set are considered changed.
//
// Returns an error if some file can't be read.
func (o Output) Compare(dir string) (changed, unchanged []string, err error) {
	for _, name := range o.Files() {
		path := filepath.Join(dir, filepath.FromSlash(name))
		existing, err := ioutil.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, nil, errors.Annotate(err, "when checking diff of %q", name).Err()
		}
		if err == nil && o.Data[name].SemanticallySame(existing) {
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
	changed, unchanged, err = o.Compare(dir)
	if err != nil {
		return
	}

	// Second pass: update changed files.
	for _, name := range changed {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(path), 0777); err != nil {
			return
		}
		if err = ioutil.WriteFile(path, o.Data[name].Bytes(), 0666); err != nil {
			return
		}
	}

	return
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
		fmt.Print(string(o.Data[f].Bytes()))
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
			o.Data[path] = BlobDatum(body)
		case os.IsNotExist(err):
			delete(o.Data, path)
		case err != nil:
			return errors.Annotate(err, "when discarding changes to %s", path).Err()
		}
	}

	return nil
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
	_, msg := v.(*starlarkprotov2.Message)
	if !str && !msg {
		return fmt.Errorf("output set value should be either a string or a proto message, not %s", v.Type())
	}

	return o.Dict.SetKey(k, v)
}

// finalize returns all output files in a single map.
//
// Protos are eventually serialized to text proto format (optionally with the
// header that tells how the file was generated).
//
// Configs supplied as strings are serialized using UTF-8 encoding.
func (o *outputBuilder) finalize(includePBHeader bool) (map[string]Datum, error) {
	out := make(map[string]Datum, o.Len())

	for _, kv := range o.Items() {
		k, v := kv[0].(starlark.String), kv[1]

		if s, ok := v.(starlark.String); ok {
			out[k.GoString()] = BlobDatum(s.GoString())
			continue
		}

		msg := v.(*starlarkprotov2.Message)
		blob, err := starlarkprotov2.ToTextPB(msg)
		if err != nil {
			return nil, err
		}

		// TODO(vadimsh): Use MessageDatum instead when it exists.
		buf := bytes.Buffer{}
		if includePBHeader {
			buf.WriteString("# Auto-generated by lucicfg.\n")
			buf.WriteString("# Do not modify manually.\n")
			if msgName, docURL := protoMessageDoc(msg); docURL != "" {
				buf.WriteString("#\n")
				fmt.Fprintf(&buf, "# For the schema of this file, see %s message:\n", msgName)
				fmt.Fprintf(&buf, "#   %s\n", docURL)
			}
			buf.WriteString("\n")
		}
		buf.Write(blob)

		out[k.GoString()] = BlobDatum(buf.Bytes())
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
