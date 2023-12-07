// Copyright 2022 The LUCI Authors.
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

// Datatool is a program which allows you to encode/decode quotapb protobuf
// messages to/from a variety of codecs.
//
// In particular, this allows you to decode lua's crazy decimal escape codes
// and also visualize the msgpackpb codec data (which is not JSON compatible,
// so most msgpack viewers will show something inaccurate).
//
// This is meant to be an internal debugging tool for server/quota developers.
package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/vmihailenco/msgpack/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/iotools"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/proto/msgpackpb"

	_ "go.chromium.org/luci/server/quota/quotapb"
)

var typeName = flag.String("type", "",
	"The quotapb message name to encode/decode.\nThis can either be a full proto message name (like 'google.protobuf.Duration'),\nor it will be looked up relative to the quotapb package (like 'Account').")

var inCodec = flag.String("in", "jsonpb", "The format for reading data from stdin.")
var outCodec = flag.String("out", "jsonpb", "The format for writing data to stdout.")

var forceRoundTrip = flag.Bool("force", false, "Force round-trip through proto.")

type codecImpl interface {
	encode(proto.Message, io.Writer) error
	decode(proto.Message, io.Reader) error
}

type msgpackDecoder interface {
	decodeToMsgpack(io.Reader) (msgpack.RawMessage, error)
}
type msgpackEncoder interface {
	encodeFromMsgpack(msgpack.RawMessage, io.Writer) error
}

type codec struct {
	blurb string
	impl  codecImpl
}

var codecs = map[string]codec{}

type jsonpbCodec struct{}

func (jsonpbCodec) encode(msg proto.Message, w io.Writer) error {
	raw, err := protojson.MarshalOptions{
		Multiline:     true,
		UseProtoNames: true,
	}.Marshal(msg)
	if err == nil {
		_, err = w.Write(raw)
	}
	return err
}
func (jsonpbCodec) decode(msg proto.Message, r io.Reader) error {
	raw, err := io.ReadAll(r)
	if err == nil {
		err = protojson.Unmarshal(raw, msg)
	}
	return err
}
func init() {
	codecs["jsonpb"] = codec{"JSONPB encoding", jsonpbCodec{}}
}

type pbCodec struct{}

func (pbCodec) encode(msg proto.Message, w io.Writer) error {
	dat, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = w.Write(dat)
	return err
}
func (pbCodec) decode(msg proto.Message, r io.Reader) error {
	dat, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return proto.Unmarshal(dat, msg)
}
func init() {
	codecs["pb"] = codec{"protobuf encoding (binary)", pbCodec{}}
}

type msgpackpbCodec struct{}

func (msgpackpbCodec) encode(msg proto.Message, w io.Writer) error {
	return msgpackpb.MarshalStream(w, msg, msgpackpb.Deterministic)
}
func (msgpackpbCodec) decode(msg proto.Message, r io.Reader) error {
	return msgpackpb.UnmarshalStream(r, msg)
}
func (msgpackpbCodec) decodeToMsgpack(r io.Reader) (msgpack.RawMessage, error) {
	ret, err := io.ReadAll(r)
	if err == nil {
		return msgpack.RawMessage(ret), nil
	}
	return nil, err
}
func init() {
	codecs["msgpackpb"] = codec{"msgpackpb encoding (binary).", msgpackpbCodec{}}
}

type msgpackPrettyCodec struct{}

func prettyPrintMsgpack(w io.Writer, indent string, obj any) {
	ws := func(fmtStr string, args ...any) {
		fmt.Fprintf(w, fmtStr, args...)
	}

	switch x := obj.(type) {
	case []any:
		ws("[\n")
		newIndent := indent + "  "
		for _, itm := range x {
			ws(newIndent)
			prettyPrintMsgpack(w, newIndent, itm)
			ws(",\n")
		}
		ws("%s]", indent)
	case map[any]any:
		ws("{\n")
		newIndent := indent + "  "
		for key, itm := range x {
			ws(newIndent)
			prettyPrintMsgpack(w, newIndent, key)
			ws(": ")
			prettyPrintMsgpack(w, newIndent, itm)
			ws(",\n")
		}
		ws("%s}", indent)
	case uint8:
		ws("8u%d", x)
	case uint16:
		ws("16u%d", x)
	case uint32:
		ws("32u%d", x)
	case uint64:
		ws("64u%d", x)
	case int8:
		ws("8i%d", x)
	case int16:
		ws("16i%d", x)
	case int32:
		ws("32i%d", x)
	case int64:
		ws("64i%d", x)
	case bool:
		ws("%t", x)
	case float32:
		ws("32f%f", x)
	case float64:
		ws("64f%f", x)
	case []byte:
		ws("b%q", x)
	case string:
		ws("%q", x)
	default:
		panic(fmt.Sprintf("unknown msgback primitive: %T", x))
	}
}

func (m msgpackPrettyCodec) encode(msg proto.Message, w io.Writer) error {
	raw, err := msgpackpb.Marshal(msg, msgpackpb.Deterministic)
	if err == nil {
		err = m.encodeFromMsgpack(raw, w)
	}
	return err
}
func (msgpackPrettyCodec) encodeFromMsgpack(raw msgpack.RawMessage, w io.Writer) error {
	dec := msgpack.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.SetMapDecoder(func(d *msgpack.Decoder) (any, error) {
		return d.DecodeUntypedMap()
	})

	var iface any
	iface, err := dec.DecodeInterface()
	if err != nil {
		return err
	}

	prettyPrintMsgpack(w, "", iface)
	_, _ = w.Write([]byte("\n"))
	return nil
}
func (msgpackPrettyCodec) decode(msg proto.Message, r io.Reader) error {
	return errors.New("msgpackpb+pretty does not support input")
}
func init() {
	codecs["msgpackpb+pretty"] = codec{"Output only; For debugging msgpack structure detail.", msgpackPrettyCodec{}}
}

type msgpackpbLuaCodec struct{}

type luaWriter struct{ w io.Writer }

var _ io.Writer = &luaWriter{}

func (l *luaWriter) Write(buf []byte) (n int, err error) {
	for _, b := range buf {
		if b == '\\' {
			_, _ = l.w.Write([]byte(`\\`))
		} else if b >= ' ' && b <= '~' {
			_, _ = l.w.Write([]byte{b})
		} else {
			fmt.Fprintf(l.w, "\\%03d", b)
		}
		n++
	}
	return
}

func (msgpackpbLuaCodec) encode(msg proto.Message, w io.Writer) error {
	return msgpackpb.MarshalStream(&luaWriter{w}, msg, msgpackpb.Deterministic)
}

var escapes = regexp.MustCompile(`(\\\\)|(\\\d\d\d)`)
var literalSlash = []byte(`\\`)

func (m msgpackpbLuaCodec) decode(msg proto.Message, r io.Reader) error {
	raw, err := m.decodeToMsgpack(r)
	if err == nil {
		err = msgpackpb.Unmarshal(raw, msg)
	}
	return err
}

func (msgpackpbLuaCodec) decodeToMsgpack(r io.Reader) (msgpack.RawMessage, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	raw = bytes.Trim(raw, `"'`)
	raw = escapes.ReplaceAllFunc(raw, func(b []byte) []byte {
		if err != nil {
			return nil
		}
		if bytes.Equal(b, literalSlash) {
			return []byte(`\`)
		}
		var byt uint64
		byt, err = strconv.ParseUint(string(b[1:]), 10, 8)
		return []byte{byte(byt)}
	})
	if err != nil {
		return nil, err
	}
	return msgpack.RawMessage(raw), nil
}
func init() {
	codecs["msgpackpb+lua"] = codec{"msgpackpb encoding (decimal lua string).", msgpackpbLuaCodec{}}
}

type msgpackpbGoBytesCodec struct{}

func (msgpackpbGoBytesCodec) encode(msg proto.Message, w io.Writer) error {
	raw, err := msgpackpb.Marshal(msg, msgpackpb.Deterministic)
	if err != nil {
		return err
	}
	fmt.Fprint(w, []byte(raw))
	return nil
}
func (m msgpackpbGoBytesCodec) decode(msg proto.Message, r io.Reader) error {
	raw, err := m.decodeToMsgpack(r)
	if err != nil {
		return err
	}
	return msgpackpb.Unmarshal(raw, msg)
}
func (msgpackpbGoBytesCodec) decodeToMsgpack(r io.Reader) (msgpack.RawMessage, error) {
	scn := bufio.NewScanner(r)
	scn.Split(bufio.ScanWords)

	buf := []byte{}

	for scn.Scan() {
		tok := strings.Trim(scn.Text(), "[]")
		val, err := strconv.ParseUint(tok, 10, 8)
		if err != nil {
			return nil, err
		}
		buf = append(buf, byte(val))
	}

	return msgpack.RawMessage(buf), nil
}
func init() {
	codecs["msgpackpb+gobytes"] = codec{"msgpackpb encoding (Go `[]byte` decimal format).", msgpackpbGoBytesCodec{}}
}

type msgpackpbHexStringCodec struct{}

func (msgpackpbHexStringCodec) encode(msg proto.Message, w io.Writer) error {
	raw, err := msgpackpb.Marshal(msg, msgpackpb.Deterministic)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%q", string(raw))
	return nil
}
func (m msgpackpbHexStringCodec) decode(msg proto.Message, r io.Reader) error {
	raw, err := m.decodeToMsgpack(r)
	if err != nil {
		return err
	}
	return msgpackpb.Unmarshal(raw, msg)
}
func (msgpackpbHexStringCodec) decodeToMsgpack(r io.Reader) (msgpack.RawMessage, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	rawS, err := strconv.Unquote(string(raw))
	if err != nil {
		return nil, err
	}
	return msgpack.RawMessage(rawS), nil
}
func init() {
	codecs["msgpackpb+hex"] = codec{"msgpackpb encoding (raw redis string).", msgpackpbHexStringCodec{}}
}

func init() {
	codecNames := []string{}
	for k := range codecs {
		codecNames = append(codecNames, k)
	}
	sort.Strings(codecNames)

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "\n")
		fmt.Fprintf(flag.CommandLine.Output(),
			"This program transforms quotapb proto messages from one form to another.\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Valid formats are:\n")
		for _, name := range codecNames {
			fmt.Fprintf(flag.CommandLine.Output(), "  %s - %s\n", name, codecs[name].blurb)
		}
		fmt.Fprintf(flag.CommandLine.Output(), "\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Flags:\n")
		flag.PrintDefaults()
	}
}

func main() {
	flag.Parse()

	ctx := gologger.StdConfig.Use(context.Background())

	if *typeName == "" {
		flag.Usage()
		logging.Errorf(ctx, "-type is required")
		os.Exit(1)
	}
	fullName := protoreflect.FullName("go.chromium.org.luci.server.quota.quotapb." + *typeName)
	mt, err := protoregistry.GlobalTypes.FindMessageByName(fullName)
	if err == protoregistry.NotFound {
		mt, err = protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(*typeName))
		if err != nil {
			logging.Errorf(ctx, "could not load type %q: %s", *typeName, err)
			os.Exit(1)
		}
	} else if err != nil {
		logging.Errorf(ctx, "could not load type %q: %s", fullName, err)
		os.Exit(1)
	}

	cIn, ok := codecs[*inCodec]
	if !ok {
		flag.Usage()
		logging.Errorf(ctx, "invalid -in codec: %q", *inCodec)
		os.Exit(1)
	}
	cOut, ok := codecs[*outCodec]
	if !ok {
		flag.Usage()
		logging.Errorf(ctx, "invalid -out codec: %q", *outCodec)
		os.Exit(1)
	}

	stdout := bufio.NewWriter(os.Stdout)
	defer func() {
		if err := stdout.Flush(); err != nil {
			panic(err)
		}
	}()

	mpd, _ := cIn.impl.(msgpackDecoder)
	mpe, _ := cOut.impl.(msgpackEncoder)
	if !*forceRoundTrip && mpd != nil && mpe != nil {
		// In this case we can skip the proto message.
		// going on.
		mpk, err := mpd.decodeToMsgpack(bufio.NewReader(os.Stdin))
		if err != nil {
			logging.Errorf(ctx, "parsing stdin as encoded messagepack: %s", err)
			os.Exit(1)
		}

		_, err = iotools.WriteTracker(stdout, func(w io.Writer) error {
			return mpe.encodeFromMsgpack(mpk, w)
		})
		if err != nil {
			logging.Errorf(ctx, "encoding messagepack: %s", err)
			os.Exit(1)
		}
	} else {
		msg := mt.New().Interface()
		if err := cIn.impl.decode(msg, bufio.NewReader(os.Stdin)); err != nil {
			logging.Errorf(ctx, "failed to decode stdin: %s", err)
			os.Exit(1)
		}

		_, err = iotools.WriteTracker(stdout, func(w io.Writer) error {
			return cOut.impl.encode(msg, w)
		})
		if err != nil {
			logging.Errorf(ctx, "failed to encode to stdout: %s", err)
			os.Exit(1)
		}
	}
}
