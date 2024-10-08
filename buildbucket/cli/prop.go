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

package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/data/stringset"
)

type propertiesFlag struct {
	props         *structpb.Struct
	first         bool
	explicitlySet stringset.Set
}

// PropertiesFlag returns a flag.Getter that can read property values into props.
//
// If a flag value starts with @, properties are read from the JSON file at the
// path that follows @. Example:
//
//	-p @my_properties.json
//
// This form can be used only in the first flag value.
//
// Otherwise, a flag value must have name=value form.
// If the property value is valid JSON, then it is parsed as JSON;
// otherwise treated as a string. Example:
//
//	-p foo=1 -p 'bar={"a": 2}'
//
// Different property names can be specified multiple times.
//
// Panics if props is nil.
// String() of the return value panics if marshaling of props to JSON fails.
func PropertiesFlag(props *structpb.Struct) flag.Getter {
	if props == nil {
		panic("props is nil")
	}
	return &propertiesFlag{
		props: props,
		first: true,
		// We don't expect a lot of properties.
		// After a certain number, it is easier to pass properties via a file.
		// Choose an arbitrary number.
		explicitlySet: stringset.New(4),
	}
}

func (f *propertiesFlag) String() string {
	// https://godoc.org/flag#Value says that String() may be called with a
	// zero-valued receiver.
	if f == nil || f.props == nil {
		return ""
	}

	ret, err := (&jsonpb.Marshaler{}).MarshalToString(f.props)
	if err != nil {
		panic(err)
	}
	return ret
}

func (f *propertiesFlag) Get() any {
	return f.props
}

func (f *propertiesFlag) Set(s string) error {
	first := f.first
	f.first = false

	if strings.HasPrefix(s, "@") {
		if !first {
			return fmt.Errorf("value that starts with @ must be the first value for the flag")
		}
		fileName := s[1:]

		file, err := os.Open(fileName)
		if err != nil {
			return err
		}
		defer file.Close()
		return jsonpb.Unmarshal(file, f.props)
	}

	parts := strings.SplitN(s, "=", 2)
	if len(parts) == 1 {
		return fmt.Errorf("invalid property %q: no equals sign", s)
	}
	name := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	if f.explicitlySet.Has(name) {
		return fmt.Errorf("duplicate property %q", name)
	}
	f.explicitlySet.Add(name)

	// Try parsing as JSON.
	// Note: jsonpb cannot unmarshal structpb.Value from JSON natively,
	// so we have to wrap JSON value in an object.
	wrappedJSON := fmt.Sprintf(`{"a": %s}`, value)
	buf := &structpb.Struct{}
	if f.props.Fields == nil {
		f.props.Fields = map[string]*structpb.Value{}
	}
	if err := protojson.Unmarshal([]byte(wrappedJSON), buf); err == nil {
		f.props.Fields[name] = buf.Fields["a"]
	} else {
		// Treat as string.
		f.props.Fields[name] = &structpb.Value{
			Kind: &structpb.Value_StringValue{StringValue: value},
		}
	}
	return nil
}
