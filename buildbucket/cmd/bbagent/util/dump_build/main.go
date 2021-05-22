// Copyright 2021 The LUCI Authors.
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

package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	protov1 "github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
)

func exit(err error) {
	fmt.Fprintln(os.Stdout, err)
	os.Exit(1)
}

func main() {
	var base64decode bool
	flag.BoolVar(&base64decode, "base64", false, "require base64 decoding")
	var decompress bool
	flag.BoolVar(&decompress, "decompress", false, "require zlib decompressing")
	var legacyProto bool
	flag.BoolVar(&decompress, "legacy_proto", false, "use legacy proto lib")

	flag.Parse()

	var data []byte
	if len(flag.Args()) == 1 {
		data = []byte(flag.Arg(0))
	} else {
		var err error
		data, err = ioutil.ReadAll(os.Stdin)
		if err != nil {
			exit(errors.Annotate(err, "read from stdin").Err())
		}
	}

	if base64decode {
		copy := data
		data = data[:0]
		if _, err := base64.RawStdEncoding.Decode(copy, data); err != nil {
			exit(errors.Annotate(err, "base64 decoding").Err())
		}
	}

	if decompress {
		copy := data
		decompressing, err := zlib.NewReader(bytes.NewReader(copy))
		if err != nil {
			exit(errors.Annotate(err, "create zlib reader").Err())
		}
		data, err = ioutil.ReadAll(decompressing)
		if err != nil {
			exit(errors.Annotate(err, "read decompressing data").Err())
		}

	}
	b := &bbpb.Build{}

	if legacyProto {
		if err := protov1.Unmarshal(data, b); err != nil {
			exit(errors.Annotate(err, "legacy proto unmarshal").Err())
		}
	} else {
		if err := proto.Unmarshal(data, b); err != nil {
			exit(errors.Annotate(err, "proto unmarshal").Err())
		}
	}

	m := &protojson.MarshalOptions{
		Indent: "  ",
	}
	jsonBytes, err := m.Marshal(b)
	if err != nil {
		exit(errors.Annotate(err, "json unmarshal").Err())
	}
	fmt.Fprint(os.Stdout, string(jsonBytes))
}
