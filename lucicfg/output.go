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
	"sync"

	"go.starlark.net/starlark"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/dynamicpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/textpb"
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
	Bytes() ([]byte, error)
	// SemanticallySame is true if 'other' represents the same datum.
	SemanticallySame(other []byte) bool
}

// BlobDatum is a Datum which is just a raw byte blob.
type BlobDatum []byte

// Bytes is a raw file body to put on disk.
func (b BlobDatum) Bytes() ([]byte, error) { return b, nil }

// SemanticallySame is true if 'other == b'.
func (b BlobDatum) SemanticallySame(other []byte) bool { return bytes.Equal(b, other) }

// MessageDatum is a Datum constructed from a proto message.
type MessageDatum struct {
	Header  string
	Message *starlarkproto.Message

	// Cache proto.Message and serialized representations, since we need them
	// in multiple places: when constructing ConfigSet for sending to the
	// validation, when comparing with configs on disk and when writing them
	// to disk.
	once sync.Once
	pmsg proto.Message
	blob []byte
	err  error
}

// ensureConverted populates `pmsg` and `blob`.
func (m *MessageDatum) ensureConverted() error {
	m.once.Do(func() {
		// Grab it as proto.Message for comparisons in SemanticallySame.
		m.pmsg = m.Message.ToProto()

		// And convert to a text for strict comparisons and the final output.
		opts := prototext.MarshalOptions{
			AllowPartial: true,
			Indent:       " ",
			Resolver:     m.Message.MessageType().Loader().Types(), // used for google.protobuf.Any fields
		}
		blob, err := opts.Marshal(m.pmsg)
		if err == nil {
			blob, err = textpb.Format(blob, m.Message.MessageType().Descriptor())
		}

		if err != nil {
			m.err = err
		} else {
			m.blob = make([]byte, 0, len(m.Header)+len(blob))
			m.blob = append(m.blob, m.Header...)
			m.blob = append(m.blob, blob...)
		}
	})
	return m.err
}

// Bytes is a raw file body to put on disk.
func (m *MessageDatum) Bytes() ([]byte, error) {
	if err := m.ensureConverted(); err != nil {
		return nil, err
	}
	return m.blob, nil
}

// SemanticallySame is true if 'other' deserializes and equals b.Message.
//
// On deserialization or comparison errors returns false.
func (m *MessageDatum) SemanticallySame(other []byte) bool {
	// Convert `other` to a proto.Message.
	otherpb := dynamicpb.NewMessage(m.Message.MessageType().Descriptor())
	opts := prototext.UnmarshalOptions{
		AllowPartial: true,
		Resolver:     m.Message.MessageType().Loader().Types(), // used for google.protobuf.Any fields
	}
	if err := opts.Unmarshal(other, otherpb); err != nil {
		return false // e.g. the schema has changed or the file is totally bogus
	}

	// Convert `m.Message` to a proto.Message too.
	if err := m.ensureConverted(); err != nil {
		return false // note: we'll fail later in the serialization anyway
	}

	// Compare them semantically as protos.
	return proto.Equal(m.pmsg, otherpb)
}

// ConfigSets partitions this output into 0 or more config sets based on Roots.
//
// Returns an error if some output Datum can't be serialized.
func (o Output) ConfigSets() ([]ConfigSet, error) {
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
				var err error
				if files[f[len(root):]], err = body.Bytes(); err != nil {
					return nil, errors.Annotate(err, "serializing %s", f).Err()
				}
			}
		}

		cs[i] = ConfigSet{Name: nm, Data: files}
	}

	return cs, nil
}

// Compare compares files on disk to what's in the output.
//
// Returns names of files that are different ('changed') and same ('unchanged').
//
// If 'semantic' is true, for output files based on proto messages uses semantic
// comparison first (i.e. loads the file on disk as a proto message and compares
// it to the output message). If semantic comparisons says the files are same,
// we are done. Otherwise we still compare files as byte blobs: it is possible
// for two semantically different protos to serialize into exact same byte blobs
// (for one, this can happen when using float64 fields). We should treat such
// protos as equal, since in the end of the day we care only about how generated
// files look on the disk.
//
// If 'semantic' is false, just compares files as byte blobs.
//
// Files on disk that are not in the output set are totally ignored. Missing
// files that are listed in the output set are considered changed.
//
// Returns an error if some file on disk can't be read or some output file can't
// be serialized.
func (o Output) Compare(dir string, semantic bool) (changed, unchanged []string, err error) {
	checkSame := func(d Datum, b []byte) (bool, error) {
		if semantic {
			if d.SemanticallySame(b) {
				return true, nil
			}
			// Fallback to check protos as byte blobs, see the explanation in the
			// function comment above.
		}
		a, err := d.Bytes()
		return bytes.Equal(a, b), err
	}

	for _, name := range o.Files() {
		path := filepath.Join(dir, filepath.FromSlash(name))

		same := true
		switch existing, err := ioutil.ReadFile(path); {
		case os.IsNotExist(err):
			same = false // new output file
		case err != nil:
			return nil, nil, errors.Annotate(err, "when checking diff of %q", name).Err()
		default:
			if same, err = checkSame(o.Data[name], existing); err != nil {
				return nil, nil, errors.Annotate(err, "when checking diff of %q", name).Err()
			}
		}

		if same {
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
// up-to-date.
//
// When comparing files does byte-to-byte comparison, not a semantic one. This
// ensures all written files are formatted in a consistent style. Use Compare
// to explicitly check the semantic difference.
//
// Creates missing directories. Not atomic. All files have mode 0666.
func (o Output) Write(dir string) (changed, unchanged []string, err error) {
	// First pass: populate 'changed' and 'unchanged', so we have a valid result
	// even when failing midway through writes.
	changed, unchanged, err = o.Compare(dir, false)
	if err != nil {
		return
	}

	// Second pass: update changed files.
	for _, name := range changed {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(path), 0777); err != nil {
			return
		}
		var blob []byte
		if blob, err = o.Data[name].Bytes(); err != nil {
			return
		}
		if err = ioutil.WriteFile(path, blob, 0666); err != nil {
			return
		}
	}

	return
}

// Read replaces values in o.Data by reading them from disk as blobs.
//
// Returns an error if some file can't be read.
func (o Output) Read(dir string) error {
	for name := range o.Data {
		path := filepath.Join(dir, filepath.FromSlash(name))
		blob, err := ioutil.ReadFile(path)
		if err != nil {
			return errors.Annotate(err, "reading %q", name).Err()
		}
		o.Data[name] = BlobDatum(blob)
	}
	return nil
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
		if blob, err := o.Data[f].Bytes(); err == nil {
			fmt.Print(string(blob))
		} else {
			fmt.Printf("ERROR: %s\n", err)
		}
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
	key, ok := k.(starlark.String)
	if !ok {
		return fmt.Errorf("output set key should be a string, not %s", k.Type())
	}

	// Paths must be within the config output directory, "../" is not allowed.
	if _, err := cleanRelativePath("", key.GoString(), false); err != nil {
		return err
	}

	_, str := v.(starlark.String)
	_, msg := v.(*starlarkproto.Message)
	if !str && !msg {
		return fmt.Errorf("output set value should be either a string or a proto message, not %s", v.Type())
	}

	// Use the exact same key (not a version cleaned by cleanRelativePath), so
	// that Starlark code can read the value back using whatever dirty key it
	// used. We do the final cleanup of keys in finalize(...).
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

		key, err := cleanRelativePath("", k.GoString(), false)
		if err != nil {
			panic(err) // already validated in SetKey
		}

		if s, ok := v.(starlark.String); ok {
			out[key] = BlobDatum(s.GoString())
			continue
		}

		md := &MessageDatum{Message: v.(*starlarkproto.Message)}
		if includePBHeader {
			buf := strings.Builder{}
			buf.WriteString("# Auto-generated by lucicfg.\n")
			buf.WriteString("# Do not modify manually.\n")
			if msgName, docURL := protoMessageDoc(md.Message); docURL != "" {
				buf.WriteString("#\n")
				fmt.Fprintf(&buf, "# For the schema of this file, see %s message:\n", msgName)
				fmt.Fprintf(&buf, "#   %s\n", docURL)
			}
			buf.WriteString("\n")
			md.Header = buf.String()
		}
		out[key] = md
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
