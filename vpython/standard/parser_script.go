// Copyright 2026 The LUCI Authors.
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

package standard

import (
	"bytes"
	"io"
	"os"
	"regexp"

	"github.com/BurntSushi/toml"

	"go.chromium.org/luci/common/errors"
)

// Reference specifications: https://packaging.python.org/en/latest/specifications/inline-script-metadata/
//
// NOTE: We do NOT use the literal canonical regular expression from the specifications page verbatim.
// The canonical regex uses PCRE-specific line-ending anchors ('$\s') which are not fully compatible
// with Go's RE2 engine, and fails to parse Windows CRLF ('\r\n') line breaks safely. This pattern is
// the exact Go/RE2 high-fidelity equivalent, anchored to the start of the file (\A) to automatically
// validate the comments-only PEP 723 placement rule inside the matching engine.
// The first additional block asserts that the config is indeed at head and starts with correct prefix.
var pep723Regex = regexp.MustCompile(`(?m)\A(?P<prefix>(?:^(?:[ \t]*|[ \t]*#.*)\r?\n)*)^# /// (?P<type>[a-zA-Z0-9-]+)\r?\n(?P<content>(?:^#(| .*)\r?\n)+)^# ///\r?$`)

var (
	pep723typeIdx         = pep723Regex.SubexpIndex("type")
	pep723contentIdx      = pep723Regex.SubexpIndex("content")
	pep723TypeStartIdx    = 2 * pep723typeIdx
	pep723TypeEndIdx      = 2*pep723typeIdx + 1
	pep723ContentStartIdx = 2 * pep723contentIdx
	pep723ContentEndIdx   = 2*pep723contentIdx + 1
)

// ParseScriptMetadata parses a PEP 723 inline metadata block from a file.
// Returns (nil, nil) if no metadata block is found.
func ParseScriptMetadata(path string) (*ProjectSpec, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Fmt("failed to open script file at: %s: %w", path, err)
	}

	return parseScriptMetadataContent(content)
}

// parseScriptMetadataReader scans and parses a generic io.Reader for PEP 723 metadata blocks.
func parseScriptMetadataReader(r io.Reader) (*ProjectSpec, error) {
	content, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.Fmt("error scanning script content: %w", err)
	}
	return parseScriptMetadataContent(content)
}

// parseScriptMetadataContent parses PEP 723 metadata from raw content.
func parseScriptMetadataContent(content []byte) (*ProjectSpec, error) {
	if err := preValidateSentinels(content); err != nil {
		return nil, err
	}

	match := pep723Regex.FindSubmatchIndex(content)
	if match == nil {
		return nil, nil
	}

	// Extract submatch groups using dynamically initialized package-level group index offsets.
	typeStart, typeEnd := match[pep723TypeStartIdx], match[pep723TypeEndIdx]
	contentStart, contentEnd := match[pep723ContentStartIdx], match[pep723ContentEndIdx]

	blockType := content[typeStart:typeEnd]
	if !bytes.Equal(blockType, []byte("script")) {
		return nil, nil
	}

	rawContent := content[contentStart:contentEnd]
	tomlBytes := stripCommentPrefixes(rawContent)

	var rawSpec struct {
		RequiresPython string   `toml:"requires-python"`
		Dependencies   []string `toml:"dependencies"`
	}
	if err := toml.Unmarshal(tomlBytes, &rawSpec); err != nil {
		return nil, errors.Fmt("failed to decode PEP 723 TOML schema: %w", err)
	}

	spec := &ProjectSpec{
		Name:           "",
		RequiresPython: rawSpec.RequiresPython,
		Dependencies:   rawSpec.Dependencies,
	}

	// If a block was present but has no valid keys, treat as empty
	if spec.RequiresPython == "" && len(spec.Dependencies) == 0 {
		return nil, nil
	}

	return spec, nil
}

// preValidateSentinels parses and pre-validates block sentinels positions to throw exact diagnostic errors.
func preValidateSentinels(content []byte) error {
	var (
		inBlock       bool
		hasCodeBefore bool
	)

	// Trim single trailing newline to prevent standard split iterators from yielding an empty trailing line at EOF.
	content = bytes.TrimSuffix(content, []byte("\n"))
	content = bytes.TrimSuffix(content, []byte("\r"))

	for line := range bytes.SplitSeq(content, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if !inBlock {
			if len(trimmed) == 0 {
				continue
			}
			if bytes.HasPrefix(trimmed, []byte("#")) {
				if bytes.Equal(trimmed, []byte("# /// script")) {
					if hasCodeBefore {
						return errors.New("invalid PEP 723 script metadata: block must be placed before any executable code or docstring statements")
					}
					inBlock = true
				}
				continue
			}
			// Any non-empty, non-comment line before the block is executable code or docstrings
			hasCodeBefore = true
		} else {
			if bytes.Equal(trimmed, []byte("# ///")) {
				inBlock = false
				break
			}
			// Lines inside block must be comments.
			leftTrimmed := bytes.TrimLeft(line, " \t")
			if !bytes.HasPrefix(leftTrimmed, []byte("#")) {
				displayLine := string(bytes.TrimSuffix(line, []byte("\r")))
				return errors.Fmt("invalid PEP 723 script metadata: non-comment line inside metadata block: %q", displayLine)
			}
		}
	}

	if inBlock {
		return errors.New("unterminated PEP 723 script metadata block (missing closing # /// sentinel)")
	}
	return nil
}

// stripCommentPrefixes strips the leading "# " comment characters from well-formed lines.
// Under our strict, well-formed configuration template assumptions, every valid payload line
// starts with exactly "# " (2 characters), which is stripped cleanly via fast direct slice indexing.
func stripCommentPrefixes(content []byte) []byte {
	var buf bytes.Buffer
	buf.Grow(len(content))

	for line := range bytes.Lines(content) {
		if line = bytes.TrimSpace(line); len(line) >= 2 {
			buf.Write(line[2:])
			buf.WriteByte('\n')
		}
	}
	return buf.Bytes()
}
