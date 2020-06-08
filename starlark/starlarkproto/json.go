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

package starlarkproto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// formatJSON normalizes the JSON by removing redundant spaces.
func formatJSON(in []byte) ([]byte, error) {
	f := jsonFormatter{in: json.NewDecoder(bytes.NewReader(in))}
	f.in.UseNumber() // avoid float64, pass numbers as strings instead
	if err := f.format(); err != nil {
		return nil, fmt.Errorf("when formatting JSON: %s", err)
	}
	return f.out.Bytes(), nil
}

// jsonFormatter reads a stream of tokens from a json.Decoder and produces a
// pretty-printed JSON output.
//
// Grammar of the input token stream (guaranteed by json.Decoder) is:
//   Value: Array | Object | Bool | Number | String | Null
//   Array: '[' Value* ']'
//   Object: '{' (String Value)* '}'
//
// Notice that ',' and ':' and all whitespace are omitted. We need to restore
// them back by essentially walking the AST constructed from the input stream.
// We do not materialize the AST fully in memory but just directly produce the
// formatted output while parsing the token stream using a simple recursive
// descent parser.
type jsonFormatter struct {
	in        *json.Decoder
	out       bytes.Buffer
	indentBuf []byte
}

// format reads the tokens from f.in and writes formatted output to f.out.
func (f *jsonFormatter) format() error {
	switch tok, err := f.in.Token(); {
	case err == io.EOF:
		return nil // totally empty JSON document, this if fine
	case err != nil:
		return err
	default:
		err = f.visitValue(tok)
		if err == io.EOF {
			return io.ErrUnexpectedEOF
		}
		return err
	}
}

// indent changes the current indentation level in the emitted JSON output.
func (f *jsonFormatter) indent(inc int) {
	newIdent := len(f.indentBuf) + inc
	if newIdent < 0 {
		panic("impossible negative indent")
	}
	if cap(f.indentBuf) >= newIdent {
		f.indentBuf = f.indentBuf[:newIdent]
	} else {
		f.indentBuf = bytes.Repeat([]byte{'\t'}, newIdent)
	}
}

// newLine emits \n followed by an indentation space.
func (f *jsonFormatter) newLine() {
	f.out.WriteRune('\n')
	f.out.Write(f.indentBuf)
}

// visitValue traverses a single JSON value, emitting formatted output.
func (f *jsonFormatter) visitValue(tok json.Token) error {
	switch val := tok.(type) {
	case json.Delim:
		switch val {
		case '[':
			return f.visitArray()
		case '{':
			return f.visitObject()
		default:
			// This should not be happening, JSON decoder checks validity of the
			// JSON stream.
			panic(fmt.Sprintf("unexpected delimiter %q", val))
		}
	case bool:
		if val {
			f.out.WriteString("true")
		} else {
			f.out.WriteString("false")
		}
	case json.Number:
		f.out.WriteString(string(val))
	case string:
		f.writeJSONString(val)
	case nil:
		f.out.WriteString("null")
	default:
		// This should not be happening, JSON decoder checks validity of the
		// JSON stream.
		panic(fmt.Sprintf("unexpected token %T %v", tok, tok))
	}
	return nil
}

// visitArray traverses a JSON array, emitting formatted output.
func (f *jsonFormatter) visitArray() error {
	f.out.WriteRune('[')

	first := true
	for {
		tok, err := f.in.Token()
		if err != nil {
			return err
		}

		if tok == json.Delim(']') {
			if !first {
				f.indent(-1)
				f.newLine()
			}
			f.out.WriteRune(']')
			return nil
		}

		if first {
			f.indent(1)
			f.newLine()
		} else {
			f.out.WriteRune(',')
			f.newLine()
		}
		first = false

		if err := f.visitValue(tok); err != nil {
			return err
		}
	}
}

// visitObject traverses a JSON object, emitting formatted output.
func (f *jsonFormatter) visitObject() error {
	f.out.WriteRune('{')

	first := true
	for {
		tok, err := f.in.Token()
		if err != nil {
			return err
		}

		if tok == json.Delim('}') {
			if !first {
				f.indent(-1)
				f.newLine()
			}
			f.out.WriteRune('}')
			return nil
		}

		if first {
			f.indent(1)
			f.newLine()
		} else {
			f.out.WriteRune(',')
			f.newLine()
		}
		first = false

		// Read the key.
		key, ok := tok.(string)
		if !ok {
			// This should not be happening, JSON decoder checks validity of the
			// JSON stream.
			panic(fmt.Sprintf("expecting a string key, got %T %v", tok, tok))
		}
		f.writeJSONString(key)
		f.out.WriteString(": ")

		// Read the value.
		tok, err = f.in.Token()
		if err != nil {
			return err
		}
		if err := f.visitValue(tok); err != nil {
			return err
		}
	}
}

// writeJSONString emits a JSON string literal.
func (f *jsonFormatter) writeJSONString(val string) {
	enc := json.NewEncoder(&f.out)
	enc.SetEscapeHTML(false)
	enc.Encode(val)                 // write `"<val>"\n`
	f.out.Truncate(f.out.Len() - 1) // trims \n
}
