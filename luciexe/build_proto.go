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

package luciexe

import (
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"
)

const (
	// BuildProtoLogName is the Build.Step.Log.Name for sub-lucictx programs.
	BuildProtoLogName = "$build.proto"

	// BuildProtoContentType is the ContentType of the build.proto LogDog datagram
	// stream.
	BuildProtoContentType = protoutil.BuildMediaType

	// BuildProtoZlibContentType is the ContentType of the compressed
	// build.proto LogDog datagram stream. It's the same as BuildProtoContentType
	// except it's compressed with zlib.
	BuildProtoZlibContentType = BuildProtoContentType + "; encoding=zlib"

	// BuildProtoStreamSuffix is the logdog stream name suffix for sub-lucictx
	// programs to output their build.proto stream to.
	//
	// TODO(iannucci): Maybe change protocol so that build.proto stream can be
	// opened by the invoking process instead of the invoked process.
	BuildProtoStreamSuffix = "build.proto"

	// OutputCLIArg is the CLI argument to luciexe binaries to instruct them to
	// dump their final Build message. The value of this flag must be an absolute
	// path to a file which doesn't exist in a directory which does (and which the
	// luciexe binary has access to write in). See Output*FileExt for valid
	// extensions.
	OutputCLIArg = "--output"
)

// ParseOutputFlag will parse the --output flag from a list of args.
//
// This parses the first instance of the argument the form:
//
//    --output path/to/file.ext   OR
//    --output=path/to/file.ext   OR
//    --output=
//
// The value and the remaining args will be returned. ".ext" must be one of the
// valid build file codec extensions.
//
// If the output argument is not present, `outputPath` will be
// "" and `rest` will be the unaltered args.
func ParseOutputFlag(args []string) (outputPath string, rest []string, err error) {
	var i int
	var deleteCount int

searchLoop:
	for i = range args {
		value := args[i]
		if value == OutputCLIArg {
			if len(args) > i+1 {
				outputPath = args[i+1]
				deleteCount = 2
			} else {
				err = errors.Reason("malformed %s argument: two argument form with no value", OutputCLIArg).Err()
			}
			break searchLoop
		} else if strings.HasPrefix(value, OutputCLIArg+"=") {
			outputPath = value[len(OutputCLIArg)+1:]
			deleteCount = 1
			break searchLoop
		}
	}

	if err != nil || deleteCount == 0 || outputPath == "" {
		rest = args
		return
	}
	if _, _, err = BuildFileCodec(outputPath); err != nil {
		rest = args
		return
	}

	rest = make([]string, 0, len(args)-deleteCount)
	rest = append(rest, args[:i]...)
	rest = append(rest, args[i+deleteCount:]...)
	return
}

// These file extensions are used with the `--output` flag to determine the
// output serialization type for the luciexe's final Build message.
const (
	BuildFileNone      = "<NONE>" // for empty build paths
	BuildFileBinaryExt = ".pb"
	BuildFileTextExt   = ".textpb"
	BuildFileJSONExt   = ".json"
)

// Codec is a pair of encode/decode functions for a specific output format of
// the Build message.
type Codec struct {
	Enc func(build *bbpb.Build, w io.Writer) error
	Dec func(build *bbpb.Build, r io.Reader) error
}

// BuildFileCodecs is the mapping from output extensions to their encode/decode
// functions.
//
// BuildFileNone is intentionally omitted; lookups for BuildFileCodecs[BuildFileNone]
// will return an empty codec (i.e. pair of nil functions).
var BuildFileCodecs = map[string]Codec{
	BuildFileBinaryExt: {
		Enc: func(build *bbpb.Build, w io.Writer) error {
			data, err := proto.Marshal(build)
			if err != nil {
				return err
			}
			_, err = w.Write(data)
			return err
		},
		Dec: func(build *bbpb.Build, r io.Reader) error {
			data, err := ioutil.ReadAll(r)
			if err != nil {
				return err
			}
			return proto.Unmarshal(data, build)
		},
	},

	BuildFileTextExt: {
		Enc: func(build *bbpb.Build, w io.Writer) error {
			return proto.MarshalText(w, build)
		},
		Dec: func(build *bbpb.Build, r io.Reader) error {
			data, err := ioutil.ReadAll(r)
			if err != nil {
				return err
			}
			return proto.UnmarshalText(string(data), build)
		},
	},

	BuildFileJSONExt: {
		Enc: func(build *bbpb.Build, w io.Writer) error {
			return (&jsonpb.Marshaler{
				OrigName: true,
				Indent:   "  ",
			}).Marshal(w, build)
		},
		Dec: func(build *bbpb.Build, r io.Reader) error {
			return (&jsonpb.Unmarshaler{
				AllowUnknownFields: true,
			}).Unmarshal(r, build)
		},
	},
}

// BuildFileCodec returns the file extension and the codec for the given
// buildFilePath.
//
// If this returns
//
// Returns an error if buildFilePath does not have a valid extension.
func BuildFileCodec(buildFilePath string) (ext string, codec Codec, err error) {
	if buildFilePath == "" {
		return
	}

	switch ext = path.Ext(buildFilePath); ext {
	case BuildFileBinaryExt, BuildFileTextExt, BuildFileJSONExt:
		codec = BuildFileCodecs[ext]
		return
	}

	validExts := make([]string, 0, len(BuildFileCodecs))
	for k := range BuildFileCodecs {
		validExts = append(validExts, k)
	}
	err = errors.Reason(
		"bad extension for build proto file path: %q (expected one of: %s)",
		buildFilePath, strings.Join(validExts, ", ")).Err()
	return
}

// ReadBuildFile parses a Build message from a file.
//
// This uses the file extension of buildFilePath to look up the appropriate
// codec.
//
// If buildFilePath is "", does nothing.
func ReadBuildFile(buildFilePath string) (ret *bbpb.Build, err error) {
	_, codec, err := BuildFileCodec(buildFilePath)
	if err != nil {
		return nil, err
	}
	if decFn := codec.Dec; decFn != nil {
		file, err := os.Open(buildFilePath)
		if err != nil {
			return nil, errors.Annotate(err, "opening build file %q", buildFilePath).Err()
		}
		defer file.Close()

		ret = &bbpb.Build{}
		err = errors.Annotate(decFn(ret, file), "parsing build file %q", buildFilePath).Err()
	}
	return
}

// WriteBuildFile writes a Build message to a file.
//
// This uses the file extension of buildFilePath to look up the appropriate
// codec.
//
// If buildFilePath is "", does nothing.
func WriteBuildFile(buildFilePath string, build *bbpb.Build) (err error) {
	_, codec, err := BuildFileCodec(buildFilePath)
	if err != nil {
		return err
	}

	if encFn := codec.Enc; encFn != nil {
		file, err := os.Create(buildFilePath)
		if err != nil {
			return errors.Annotate(err, "opening build file %q", buildFilePath).Err()
		}
		defer file.Close()
		err = errors.Annotate(encFn(build, file), "writing build file %q", buildFilePath).Err()
	}
	return
}

// IsMergeStep returns true iff the given step is identified as a 'merge step'.
//
// See "Recursive Invocation" in the doc for this package.
func IsMergeStep(s *bbpb.Step) bool {
	if len(s.GetLogs()) >= 1 && s.Logs[0].Name == BuildProtoLogName {
		return true
	}
	return false
}
