// Copyright 2023 The LUCI Authors.
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

//go:build !luagentest

// Package luagen implements a lua code generator for proto code.
//
// This is NOT a protoc-gen plugin, because its output format is intentionally
// quite different. Instead of generating a 1:1 proto:lua output for each .proto
// file, this generates a single, wholly standalone lua file, which includes all
// serialization logic for a transitive closure of proto messages and enums.
// This makes using the generated code substantially easier (just a single
// require), and also allows this to be used in contexts where you need protos
// defined in places where adding adjacent .lua files would be difficult (e.g.
// adding .lua files to upstream `google.protobuf` repos would be impossible,
// and forking those repos or creating a parallel proto hierarchy would be very
// annoying).
//
// Use it by adding a file e.g. `luagen.go` which looks like:
//
//	// (See NOTE below for what this build tag means)
//	//go:build luagen
//
//	package main
//
//	import (
//		"go.chromium.org/luci/common/proto/msgpackpb/luagen"
//
//		yours "your/package/with/protos"
//	)
//
//	func main() {
//		luagen.Main(
//			// Here you can include any proto messages or enums you want to
//			// specifically in the output.
//			&yours.YourMessage{},
//		  yours.ENUM_VALUE,
//		)
//	}
//
// You can then generate code for it in a go:generate comment in a normal
// package .go file like:
//
//	//go:generate go run luagen.go output.lua
//
// NOTE: The `//go:build luagen` directive in the file allows it to exist as
// a `package main` in the same directory as other .go files. You can of course
// opt to instead put the generator snippet in a different directory, if you
// choose.
package luagen

import (
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/iotools"
)

const luaMaxInt = 9007199254740991
const luaMinInt = -9007199254740991

// Main will generate a lua script with codec definitions for all of the given
// `objects` (and any messages/enums they refer to), then write the script to
// a file specified by `os.Args[1]`.
//
// This function accepts a list of proto.Message instances as well as proto
// typed Enum values (i.e. which conform to the protoreflect.Enum interface).
// Order of these arguments does not matter.
//
// # Generated File Contents
//
// The generated file will be a module which returns a single table `PB`. The
// Contents of PB are as follows:
//
// # E
//
// The `PB.E` table stores enumeration values (both numeric and string) indexed by
// the full name of the Enum type.
//
//	{
//	  ["full.Path.To.Enum"] = {
//	    ["ENUM_VALUE_NAME"] = 10,
//	    [10] = "ENUM_VALUE_NAME",
//	  }
//	}
//
// # M
//
// The `PB.M` table contains a mapping from full name of proto Message to it's
// "codec".
//
// Each codec consists of a `keys` set (the string names of each field in the
// message), the `marshal` function (which takes a previously unmarshal'd table,
// and produces a new table which, when passed to cmsgpack.pack, will be
// msgpackpb encoded), and the `unmarshal` function (which takes the
// cmsgpack.unpack'd table, and transforms it into a message).
//
// # setInternTable
//
// The generated lua has a special 'string internment' feature. Any string value
// in any decoded message can be replaced with a numeric index (1-based) into an
// internment table. When a message with a string field (containing a number on
// the wire) is encountered, the unmarshal routine will look the string up from
// the table instead.
//
// This feature is used to allow re-use of the required Redis KEYS list for
// embedded lua scripts (i.e. Redis requires pre declaring all affected keys,
// and passing them as an unencoded list of strings; By calling
// PB.setInternTable(KEYS), the encoding side can encode any of these strings
// in the proto messages as indexes in KEYS instead).
//
// This feature is ONLY used for unmarshaling; The generated marshal code cannot
// produce msgpackpb messages with interned strings.
//
// # marhsal
//
// This is the main marshaling function; Called as `PB.marshal(obj)`, this takes
// a previously-unmarshaled message table, and returns it in msgpackpb encoding
// as a string.
//
// # unmarshal
//
// This is the main unmarshaling function; Called as `PB.unmarshal(messageName,
// msgpackpb)`, this takes a fully qualified proto message name (i.e. key in the
// `M` table), and the msgpackpb encoded data, and returns a table which is the
// decoded message. Note that the returned table will have all unset fields
// populated with the following defaults:
//
//	string - ""
//	repeated X - {}
//	bool - false
//	all number types - 0
//	message types - nil (note! lua does not preserve these values!)
//	enum types - the zero-value enum string name
//
// Unmarshaled messages also contain a "$type" field which is the fully
// qualified message name, and also an "$unknown" field, which is a table
// mapping from field number to raw decoded msgpackpb data, for unknown fields.
//
// These unknown fields are restored verbatim on `marshal`.
//
// # new
//
// New is a function to produce a brand new message, and is called like
// `PB.new(messageName, defaults)`, where `defaults` is an optional table of
// default field values to fill. `PB.new` will currently check the keys in
// `defaults` (to avoid typos), but will not check types (bad types will cause
// an error when calling `marshal` eventually.
//
// # Example Go usage
//
//	func main() {
//		luagen.Main(
//			&examplepb.Message{},
//			examplepb.ZERO,   // just need one of the enum values
//		)
//	}
//
// # Example lua usage
//
//	local PB = loadfile('generated.lua')()
//	local myObject = PB.new('full.name.of.Message', {
//	  field = "hello"
//	})
//	# myObject == {field = "hello", enum_field = "ZERO"}
//	myObject.enum_field = PB.E['full.name.of.Enum'][3] # "THREE"
//	# OR: myObject.enum_field = "THREE"
//	# OR: myObject.enum_field = 3
//	local encoded = PB.marshal(myObject)
//	# encoded is a msgpackpb string with two fields
//	local decoded = PB.unmarshal('full.name.of.Message', encoded)
//	# decoded is {field = "hello", enum_field = "THREE"}
//
// NOTE: See `examplepb` subfolder, including its luagen.go and generated .lua
// output.
func Main(objects ...any) {
	if len(objects) == 0 {
		log.Fatal("luagen.Main: No proto messages or enums provided.")
	}

	if len(os.Args) < 2 {
		log.Fatal("luagen.Main: Expected output file as first argument.")
	}
	outFile := os.Args[1]
	if !strings.HasSuffix(outFile, ".lua") {
		log.Fatalf("luagen.Main: Output file does not end with `.lua`: %q", outFile)
	}
	outFile, err := filepath.Abs(outFile)
	if err != nil {
		log.Fatalf("luagen.Main: failed to calculate absolute path to outfile: %s", err)
	}

	var msgs []protoreflect.MessageDescriptor
	var enums []protoreflect.EnumDescriptor

	for _, obj := range objects {
		switch x := obj.(type) {
		case proto.Message:
			msgs = append(msgs, x.ProtoReflect().Descriptor())
		case protoreflect.MessageDescriptor:
			msgs = append(msgs, x)
		case protoreflect.Enum:
			enums = append(enums, x.Descriptor())
		case protoreflect.EnumType:
			enums = append(enums, x.Descriptor())
		case protoreflect.EnumDescriptor:
			enums = append(enums, x)
		default:
			log.Fatalf("luagen.Main: unknown object type %T", obj)
		}
	}

	if err := generate(outFile, msgs, enums); err != nil {
		log.Fatal(err)
	}
}

type printer struct {
	w      io.Writer
	indent string
}

func (p *printer) p(fmtS string, args ...any) {
	fmt.Fprintf(p.w, p.indent+fmtS, args...)
}

func (p *printer) pl(fmtS string, args ...any) {
	p.p(fmtS+"\n", args...)
}

func (p *printer) nl() {
	// we know that p.w is using WriteTracker and so will panic on any error, so
	// just ignore the returned one here.
	_, _ = p.w.Write([]byte("\n"))
}

func (p *printer) suite(cb func(), end string) {
	p.indent += "  "
	cb()
	p.indent = p.indent[:len(p.indent)-2]
	p.pl(end)
}

func (p *printer) curly(fmtS string, args ...any) func(cb func(), end ...string) {
	return func(f func(), end ...string) {
		if !strings.Contains(fmtS, "{") {
			fmtS = fmtS + " {"
		}
		p.pl(fmtS, args...)
		p.suite(f, strings.Join(append([]string{"}"}, end...), ""))
	}
}

func (p *printer) cond(fmtS string, args ...any) func(func()) {
	return func(f func()) {
		p.pl("if "+fmtS+" then", args...)
		p.suite(f, "end")
	}
}

func emitMarshalTypecheck(p *printer, valName string, field protoreflect.FieldDescriptor, fieldName, assignTo string) {
	pl := p.pl
	cond := p.cond
	suite := p.suite

	if assignTo == "" {
		assignTo = valName
	}

	pl("local T = type(%s)", valName)

	exType := func(luaType string) {
		cond(`T ~= "%s"`, luaType)(func() {
			pl(`error("field %s: expected %s, but got "..T)`, fieldName, luaType)
		})
	}

	switch kind := field.Kind(); kind {
	case protoreflect.BytesKind, protoreflect.StringKind:
		exType("string")
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		exType("number")
		cond(`%s > %d`, valName, math.MaxInt32)(func() { pl(`error("field %s: overflows int32")`, fieldName) })
		cond(`%s < %d`, valName, math.MinInt32)(func() { pl(`error("field %s: underflows int32")`, fieldName) })
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		exType("number")
		cond(`%s > %d`, valName, luaMaxInt)(func() { pl(`error("field %s: overflows lua max integer")`, fieldName) })
		cond(`%s < %d`, valName, luaMinInt)(func() { pl(`error("field %s: underflows lua min integer")`, fieldName) })
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		exType("number")
		cond(`%s < 0`, valName)(func() { pl(`error("field %s: negative")`, fieldName) })
		cond(`%s > %d`, valName, math.MaxUint32)(func() { pl(`error("field %s: overflows max uint32")`, fieldName) })
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		exType("number")
		cond(`%s < 0`, valName)(func() { pl(`error("field %s: negative")`, fieldName) })
		cond(`%s > %d`, valName, luaMaxInt)(func() { pl(`error("field %s: overflows lua max integer")`, fieldName) })
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		exType("number")
	case protoreflect.BoolKind:
		exType("boolean")
	case protoreflect.MessageKind:
		exType("table")
		fullName := field.Message().FullName()
		cond(`not %s["$type"]`, valName)(func() {
			pl(`error("field %s: missing type")`, fieldName)
		})
		cond(`%s["$type"] ~= %q`, valName, fullName)(func() {
			pl(`error("field %s: expected message type '%s', but got "..%s["$type"])`, fieldName, fullName, valName)
		})
		pl(`%s = PB.M[%q].marshal(%s)`, assignTo, fullName, valName)
	case protoreflect.EnumKind:
		pl("local origval = %s", valName)
		pl(`if T == "string" then`)
		suite(func() {
			pl("%s = PB.E[%q][%s]", assignTo, field.Enum().FullName(), valName)
			cond(`%s == nil`, assignTo)(func() {
				pl(`error("field %s: bad string enum value "..origval)`, fieldName)
			})
		}, `elseif T == "number" then`)
		suite(func() {
			cond(`PB.E[%q][%s] == nil`, field.Enum().FullName(), valName)(func() {
				pl(`error("field %s: bad numeric enum value "..origval)`, fieldName)
			})
		}, "else")
		suite(func() {
			pl(`error("field %s: expected number or string, but got "..T)`, fieldName)
		}, "end")
	default:
		panic(fmt.Sprintf("unsupported field kind %s", kind))
	}
}

func emitUnmarshalTypeCheck(p *printer, valName string, field protoreflect.FieldDescriptor, fieldName, assignTo string) {
	cond := p.cond
	pl := p.pl
	kind := field.Kind()

	if assignTo == "" {
		assignTo = valName
	}

	exType := func(luaType string, displayname ...string) {
		dname := luaType
		if len(displayname) > 0 {
			dname = displayname[0]
		}
		cond(`T ~= "%s"`, luaType)(func() {
			pl(`error("field %s: expected %s, but got "..T)`, fieldName, dname)
		})
	}

	pl("local T = type(%s)", valName)

	switch kind {
	case protoreflect.BytesKind, protoreflect.StringKind:
		cond(`T == "number"`)(func() {
			cond(`not PB.internUnmarshalTable`)(func() {
				pl(`error("field %s: failed to look up interned string: intern table not set")`, fieldName)
			})
			pl("local origval = %s", valName)
			pl("local newval = PB.internUnmarshalTable[%s]", valName)
			cond(`newval == nil`)(func() {
				pl(`error("field %s: failed to look up interned string: "..origval)`, fieldName)
			})
			pl("val = newval")
			pl("T = type(val)")
		})
		exType("string")
		pl("%s = val", assignTo)
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
		protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind,
		protoreflect.FloatKind, protoreflect.DoubleKind:
		exType("number")
		pl("%s = val", assignTo)
	case protoreflect.BoolKind:
		exType("boolean")
		pl("%s = val", assignTo)
	case protoreflect.MessageKind:
		exType("table")
		pl("%s = PB.M[%q].unmarshal(%s)", assignTo, field.Message().FullName(), valName)
	case protoreflect.EnumKind:
		exType("number", "numeric enum")
		pl("local origval = %s", valName)
		pl("local newval = PB.E[%q][%s]", field.Enum().FullName(), valName)
		cond(`newval == nil`)(func() {
			pl(`error("field %s: bad enum value "..origval)`, fieldName)
		})
		pl("%s = newval", assignTo)
	default:
		panic(fmt.Sprintf("unsupported field kind %s", kind))
	}
}

func kindString(fd protoreflect.FieldDescriptor) (kindString string) {
	singular := func(fd protoreflect.FieldDescriptor) (ret string) {
		kind := fd.Kind()
		if kind == protoreflect.MessageKind {
			ret = string(fd.Message().FullName())
		} else if kind == protoreflect.EnumKind {
			ret = "enum " + string(fd.Enum().FullName())
		} else {
			ret = kind.String()
		}
		if fd.IsList() {
			ret = "repeated " + ret
		}
		return
	}

	if fd.IsMap() {
		keyKind := fd.MapKey().Kind()
		return fmt.Sprintf("map<%s, %s>", keyKind, singular(fd.MapValue()))
	}
	return singular(fd)
}

func zeroValue(field protoreflect.FieldDescriptor) string {
	kind := field.Kind()
	switch kind {
	case protoreflect.BytesKind, protoreflect.StringKind:
		return `""`
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
		protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind,
		protoreflect.FloatKind, protoreflect.DoubleKind:
		return `0`
	case protoreflect.BoolKind:
		return `false`
	case protoreflect.MessageKind:
		return `nil`
	case protoreflect.EnumKind:
		return fmt.Sprintf("%q", field.Enum().Values().ByNumber(0).Name())
	default:
		panic(fmt.Sprintf("unsupported field kind %s", kind))
	}
}

func generate(outFile string, msgs []protoreflect.MessageDescriptor, enums []protoreflect.EnumDescriptor) error {
	allEnumsMap := map[string]protoreflect.EnumDescriptor{}

	addEnums := func(enum protoreflect.EnumDescriptor) {
		key := string(enum.FullName())
		if _, ok := allEnumsMap[key]; !ok {
			allEnumsMap[key] = enum
		}
	}
	for _, enum := range enums {
		addEnums(enum)
	}

	allMsgMap := map[string]protoreflect.MessageDescriptor{}

	var addMsgs func(msg protoreflect.MessageDescriptor)
	addMsgs = func(msg protoreflect.MessageDescriptor) {
		key := string(msg.FullName())
		if _, ok := allMsgMap[key]; !ok {
			allMsgMap[key] = msg
			fields := msg.Fields()
			for i := range fields.Len() {
				field := fields.Get(i)
				if field.IsMap() {
					field = field.MapValue()
				}
				if field.Kind() == protoreflect.MessageKind {
					addMsgs(field.Message())
				} else if field.Kind() == protoreflect.EnumKind {
					addEnums(field.Enum())
				}
			}
		}
	}
	for _, msg := range msgs {
		addMsgs(msg)
	}

	allMsgNames := make([]string, 0, len(allMsgMap))
	for k := range allMsgMap {
		allMsgNames = append(allMsgNames, k)
	}
	sort.Strings(allMsgNames)
	allEnumNames := make([]string, 0, len(allEnumsMap))
	for k := range allEnumsMap {
		allEnumNames = append(allEnumNames, k)
	}
	sort.Strings(allEnumNames)

	of, err := os.Create(outFile)
	if err != nil {
		return errors.Fmt("opening output file: %w", err)
	}
	defer func() {
		if err := of.Close(); err != nil {
			panic(err)
		}
	}()

	_, err = iotools.WriteTracker(of, func(w io.Writer) error {
		printerState := printer{w, ""}
		cond := printerState.cond
		curly := printerState.curly
		nl := printerState.nl
		pl := printerState.pl
		suite := printerState.suite

		pl("local PB = {}")
		nl()
		pl("local next = next") // these are more efficient than global lookup
		pl("local type = type")
		nl()

		curly("PB.E =")(func() {
			var needNl bool
			for _, key := range allEnumNames {
				enum := allEnumsMap[key]
				if needNl {
					nl()
				}
				needNl = true

				curly(`[%q] =`, key)(func() {
					vals := enum.Values()
					for i := range vals.Len() {
						val := vals.Get(i)
						pl(`[%q] = %d,`, val.Name(), val.Number())
						pl(`[%d] = %q,`, val.Number(), val.Name())
					}
				}, ",")
			}
		})

		nl()
		curly("PB.M =")(func() {
			var needNl bool
			for _, fullName := range allMsgNames {
				desc := allMsgMap[fullName]
				if needNl {
					nl()
				}
				needNl = true

				curly(`["%s"] =`, fullName)(func() {
					pl("marshal = function(obj)")
					suite(func() {
						pl("local acc, val, T = {}, nil, nil, nil")

						fields := desc.Fields()
						for i := range fields.Len() {
							field := fields.Get(i)
							name := field.Name()
							num := field.Number()
							kind := field.Kind()

							nl()
							pl("val = obj[%q] -- %d: %s", name, num, kindString(field))
							if field.IsList() {
								cond("next(val) ~= nil")(func() {
									pl("local T = type(val)")
									cond(`T ~= "table"`)(func() {
										pl(`error("field %s: expected list[%s], but got "..T)`, name, kind)
									})

									pl("local maxIdx = 0")
									pl("local length = 0")
									pl("for i, v in next, val do")
									suite(func() {
										cond(`type(i) ~= "number"`)(func() {
											pl(`error("field %s: expected list[%s], but got table")`, name, kind)
										})
										emitMarshalTypecheck(&printerState, "v", field, fmt.Sprintf(`%s["..(i-1).."]`, name), "val[i]")
										cond(`i > maxIdx`)(func() {
											pl("maxIdx = i")
										})
										pl("length = length + 1")
									}, "end")
									cond(`length ~= maxIdx`)(func() {
										pl(`error("field %s: expected list[%s], but got table")`, name, kind)
									})
									pl("acc[%d] = val", num)
								})
							} else if field.IsMap() {
								cond("next(val) ~= nil")(func() {
									pl("local T = type(val)")
									cond(`T ~= "table"`)(func() {
										pl(`error("field %s: expected map<%s, %s>, but got "..T)`, name, field.MapKey().Kind(), field.MapValue().Kind())
									})

									pl("local i = 0")
									pl("for k, v in next, val do")
									suite(func() {
										emitMarshalTypecheck(&printerState, "k", field.MapKey(), fmt.Sprintf(`%s["..i.."th entry]`, name), "")
										emitMarshalTypecheck(&printerState, "v", field.MapValue(), fmt.Sprintf(`%s["..k.."]`, name), "val[k]")
										pl("i = i + 1")
									}, "end")
									pl("acc[%d] = val", num)
								})
							} else {
								var condStr string
								if field.Kind() == protoreflect.EnumKind {
									condStr = `val ~= 0 and val ~= %s`
								} else {
									condStr = `val ~= %s`
								}
								cond(condStr, zeroValue(field))(func() {
									emitMarshalTypecheck(&printerState, "val", field, string(name), "")
									pl("acc[%d] = val", num)
								})
							}
						}

						nl()
						pl(`local unknown = obj["$unknown"]`)
						cond(`unknown ~= nil`)(func() {
							pl(`for k, v in next, unknown do acc[k] = v end`)
						})

						pl("return acc")
					}, "end,")

					nl()
					pl("unmarshal = function(raw)")
					suite(func() {
						pl("local defaults = {}")
						curly("local ret = ")(func() {
							pl(`["$unknown"] = {},`)
							pl(`["$type"] = %q,`, fullName)
							fields := desc.Fields()
							for i := range fields.Len() {
								field := fields.Get(i)
								var def string
								if field.IsList() || field.IsMap() {
									def = `{}`
								} else {
									def = zeroValue(field)
								}
								pl(`[%q] = %s,`, field.Name(), def)
							}
						})

						// generate the decoding table
						curly("local dec =")(func() {
							fields := desc.Fields()
							for i := range fields.Len() {
								field := fields.Get(i)
								name := field.Name()
								kind := field.Kind()

								pl("[%d] = function(val) -- %s: %s", field.Number(), name, kindString(field))
								suite(func() {
									if field.IsList() {
										pl("local T = type(val)")
										cond(`T ~= "table"`)(func() {
											pl(`error("field %s: expected list[%s], but got "..T)`, name, kind)
										})

										pl("local max = 0")
										pl("local count = 0")
										pl("for i, v in next, val do")
										suite(func() {
											cond(`type(i) ~= "number"`)(func() {
												pl(`error("field %s: expected list[%s], but got table")`, name, kind)
											})
											cond(`i > max`)(func() {
												pl("max = i")
											})
											pl("count = count + 1")
											emitUnmarshalTypeCheck(&printerState, "v", field, fmt.Sprintf(`%s["..(i-1).."]`, name), "val[i]")
										}, "end")
										cond(`max ~= count`)(func() {
											pl(`error("field %s: expected list[%s], but got table")`, name, kind)
										})
										pl("ret[%q] = val", name)
									} else if field.IsMap() {
										pl("local T = type(val)")
										cond(`T ~= "table"`)(func() {
											pl(`error("field %s: expected map<%s, %s>, but got "..T)`, name, field.MapKey().Kind(), field.MapValue().Kind())
										})

										pl("local i = 0")
										pl("for k, v in next, val do")
										suite(func() {
											emitUnmarshalTypeCheck(&printerState, "k", field.MapKey(), fmt.Sprintf(`%s["..i.."th entry]`, name), "")
											emitUnmarshalTypeCheck(&printerState, "v", field.MapValue(), fmt.Sprintf(`%s["..k.."]`, name), "val[k]")
											pl("i = i + 1")
										}, "end")
										pl("ret[%q] = val", name)
									} else {
										emitUnmarshalTypeCheck(&printerState, "val", field, string(name), fmt.Sprintf("ret[%q]", name))
									}

									if oneof := field.ContainingOneof(); oneof != nil {
										// this is tricky; we would need to introduce setters at the
										// very least, but probably also interrogators. Just panic
										// for now.
										panic("oneof currently not supported")
									}
								}, "end,")
							}
						})

						pl("for k, v in next, raw do")
						suite(func() {
							pl("local fn = dec[k]")
							pl("if fn ~= nil then")
							suite(func() {
								pl("fn(v)")
							}, "else")
							suite(func() {
								pl(`ret["$unknown"][k] = v`)
							}, "end")
						}, "end")

						pl("return ret")
					}, "end,")

					pl(`keys = {`)
					suite(func() {
						fields := desc.Fields()
						for i := range fields.Len() {
							pl(`[%q] = true,`, fields.Get(i).Name())
						}
					}, `},`)
				}, ",")
			}
		})

		nl()
		// we do this to allow cmsgpack to be passed in during testing, but not in
		// produciont.
		cond(`cmsgpack == nil`)(func() {
			pl(`local cmsgpack = ...`)
			pl(`assert(cmsgpack)`)
		})
		pl("local cmsgpack_pack = cmsgpack.pack")
		pl("local cmsgpack_unpack = cmsgpack.unpack")

		pl("function PB.setInternTable(t)")
		suite(func() {
			cond("PB.internUnmarshalTable")(func() {
				pl(`error("cannot set intern table twice")`)
			})
			cond(`type(t) ~= "table"`)(func() {
				pl(`error("must call PB.setInternTable with a table (got "..type(t)..")")`)
			})
			pl("PB.internUnmarshalTable = {}")
			pl("for i, v in ipairs(t) do")
			suite(func() {
				pl("PB.internUnmarshalTable[i-1] = v")
			}, "end")
		}, "end")

		pl("function PB.marshal(obj)")
		suite(func() {
			pl(`local T = obj["$type"]`)
			cond(`T == nil`)(func() {
				pl(`error("obj is missing '$type' field")`)
			})
			pl(`local codec = PB.M[T]`)
			cond(`codec == nil`)(func() {
				pl(`error("unknown proto message type: "..T)`)
			})
			pl(`return cmsgpack_pack(codec.marshal(obj))`)
		}, "end")

		pl("function PB.unmarshal(messageName, msgpackpb)")
		suite(func() {
			pl(`local codec = PB.M[messageName]`)
			cond(`codec == nil`)(func() {
				pl(`error("unknown proto message type: "..messageName)`)
			})
			pl(`return codec.unmarshal(cmsgpack_unpack(msgpackpb))`)
		}, "end")

		pl("function PB.new(messageName, defaults)")
		suite(func() {
			pl(`local codec = PB.M[messageName]`)
			cond(`codec == nil`)(func() {
				pl(`error("unknown proto message type: "..messageName)`)
			})
			pl(`local ret = codec.unmarshal({})`)
			cond(`defaults ~= nil`)(func() {
				pl(`for k, v in next, defaults do`)
				suite(func() {
					cond(`k[0] ~= "$"`)(func() {
						cond(`not codec.keys[k]`)(func() {
							pl(`error("invalid property name '"..k.."' for "..messageName)`)
						})
						pl(`ret[k] = v`)
					})
				}, "end")
			})
			pl(`return ret`)
		}, "end")

		pl("return PB")

		return nil
	})

	return err
}
