// Copyright 2020 The LUCI Authors.
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

package luciexe

import (
	"io"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
)

// BuildFileCodec represents the set of functions to properly encode and decode
// a build.proto message.
type BuildFileCodec interface {
	FileExtension() string
	IsNoop() bool
	Enc(build *bbpb.Build, w io.Writer) error
	Dec(build *bbpb.Build, r io.Reader) error
}

// Noop codec
type buildFileCodecNoop struct{}

var _ BuildFileCodec = buildFileCodecNoop{}

func (buildFileCodecNoop) FileExtension() string            { return "" }
func (buildFileCodecNoop) IsNoop() bool                     { return true }
func (buildFileCodecNoop) Enc(*bbpb.Build, io.Writer) error { return nil }
func (buildFileCodecNoop) Dec(*bbpb.Build, io.Reader) error { return nil }

// Binary protobuf codec
type buildFileCodecBinary struct{}

var _ BuildFileCodec = buildFileCodecBinary{}

func (buildFileCodecBinary) FileExtension() string { return ".pb" }
func (buildFileCodecBinary) IsNoop() bool          { return false }
func (buildFileCodecBinary) Enc(build *bbpb.Build, w io.Writer) error {
	data, err := proto.Marshal(build)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
func (buildFileCodecBinary) Dec(build *bbpb.Build, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return proto.Unmarshal(data, build)
}

// Text protobuf codec
type buildFileCodecText struct{}

var _ BuildFileCodec = buildFileCodecText{}

func (buildFileCodecText) FileExtension() string { return ".textpb" }
func (buildFileCodecText) IsNoop() bool          { return false }
func (buildFileCodecText) Enc(build *bbpb.Build, w io.Writer) error {
	return proto.MarshalText(w, build)
}
func (buildFileCodecText) Dec(build *bbpb.Build, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return proto.UnmarshalText(string(data), build)
}

// JSON protobuf codec
type buildFileCodecJSON struct{}

var _ BuildFileCodec = buildFileCodecJSON{}

func (buildFileCodecJSON) FileExtension() string { return ".json" }
func (buildFileCodecJSON) IsNoop() bool          { return false }
func (buildFileCodecJSON) Enc(build *bbpb.Build, w io.Writer) error {
	return (&jsonpb.Marshaler{
		OrigName: true,
		Indent:   "  ",
	}).Marshal(w, build)
}
func (buildFileCodecJSON) Dec(build *bbpb.Build, r io.Reader) error {
	return (&jsonpb.Unmarshaler{
		AllowUnknownFields: true,
	}).Unmarshal(r, build)
}

// These are the known BuildFileCodec implementations.
var (
	// This has 'IsNoop' set to true; the Enc/Dec functions do nothing.
	BuildFileCodecNoop BuildFileCodec = buildFileCodecNoop{}

	BuildFileCodecBinary BuildFileCodec = buildFileCodecBinary{}
	BuildFileCodecJSON   BuildFileCodec = buildFileCodecJSON{}
	BuildFileCodecText   BuildFileCodec = buildFileCodecText{}
)

var validCodecExts = map[string]BuildFileCodec{
	buildFileCodecBinary{}.FileExtension(): buildFileCodecBinary{},
	buildFileCodecJSON{}.FileExtension():   buildFileCodecJSON{},
	buildFileCodecText{}.FileExtension():   buildFileCodecText{},
}
var validCodecExtsStr string

func init() {
	exts := make([]string, 0, len(validCodecExts))
	for k := range validCodecExts {
		exts = append(exts, k)
	}
	sort.Strings(exts)
	validCodecExtsStr = strings.Join(exts, ", ")
}

// BuildFileCodecForPath returns the file BuildFileCodec for the given
// buildFilePath.
//
// If buildFilePath is empty, returns BuildFileNone.
//
// Returns an error if buildFilePath does not have a valid BuildFileExtension.
func BuildFileCodecForPath(buildFilePath string) (codec BuildFileCodec, err error) {
	if buildFilePath == "" {
		codec = buildFileCodecNoop{}
		return
	}

	if codec = validCodecExts[path.Ext(buildFilePath)]; codec != nil {
		return
	}

	err = errors.Reason(
		"bad extension for build proto file path: %q (expected one of: %s)",
		buildFilePath, validCodecExtsStr).Err()
	return
}

// ReadBuildFile parses a Build message from a file.
//
// This uses the file BuildFileExtension of buildFilePath to look up the appropriate
// codec.
//
// If buildFilePath is "", does nothing.
func ReadBuildFile(buildFilePath string) (ret *bbpb.Build, err error) {
	codec, err := BuildFileCodecForPath(buildFilePath)
	if err != nil {
		return nil, err
	}
	if !codec.IsNoop() {
		file, err := os.Open(buildFilePath)
		if err != nil {
			return nil, errors.Annotate(err, "opening build file %q", buildFilePath).Err()
		}
		defer file.Close()

		ret = &bbpb.Build{}
		if err = codec.Dec(ret, file); err != nil {
			err = errors.Annotate(err, "parsing build file %q", buildFilePath).Err()
		}
	}
	return
}

// WriteBuildFile writes a Build message to a file.
//
// This uses the file BuildFileExtension of buildFilePath to look up the appropriate
// codec.
//
// If buildFilePath is "", does nothing.
func WriteBuildFile(buildFilePath string, build *bbpb.Build) (err error) {
	codec, err := BuildFileCodecForPath(buildFilePath)
	if err != nil {
		return err
	}
	if !codec.IsNoop() {
		file, err := os.Create(buildFilePath)
		if err != nil {
			return errors.Annotate(err, "opening build file %q", buildFilePath).Err()
		}
		defer file.Close()
		if err = codec.Enc(build, file); err != nil {
			err = errors.Annotate(err, "writing build file %q", buildFilePath).Err()
		}
	}
	return
}
