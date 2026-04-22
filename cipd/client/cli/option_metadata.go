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

package cli

import (
	"bytes"
	"context"
	"flag"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

////////////////////////////////////////////////////////////////////////////////
// metadataOptions mixin.

type metadataFlagValue struct {
	key         string
	value       string // either a literal value or a path to read it from
	contentType string
}

type metadataList struct {
	entries   []metadataFlagValue
	valueKind string // "value" or "path"
}

func (md *metadataList) String() string {
	return "key:" + md.valueKind
}

// Set is called by 'flag' package when parsing command line options.
func (md *metadataList) Set(value string) error {
	// Should have form key_with_possible_content_type:value.
	chunks := strings.SplitN(value, ":", 2)
	if len(chunks) != 2 {
		return md.badFormatError()
	}

	// Extract content-type from within trailing '(...)', if present.
	key, contentType, value := chunks[0], "", chunks[1]
	switch l, r := strings.Index(key, "("), strings.LastIndex(key, ")"); {
	case l == -1 && r == -1:
		// no content type, this is fine
	case l != -1 && r != -1 && l < r:
		// The closing ')' should be the last character.
		if !strings.HasSuffix(key, ")") {
			return md.badFormatError()
		}
		key, contentType = key[:l], key[l+1:r]
	default:
		return md.badFormatError()
	}

	// Validate everything we can.
	if err := common.ValidateInstanceMetadataKey(key); err != nil {
		return cliErrorTag.Apply(err)
	}
	if err := common.ValidateContentType(contentType); err != nil {
		return cliErrorTag.Apply(err)
	}
	if md.valueKind == "value" {
		if err := common.ValidateInstanceMetadataLen(len(value)); err != nil {
			return cliErrorTag.Apply(err)
		}
	}

	md.entries = append(md.entries, metadataFlagValue{
		key:         key,
		value:       value,
		contentType: contentType,
	})
	return nil
}

func (md *metadataList) badFormatError() error {
	return makeCLIError("should have form key:%s or key(content-type):%s", md.valueKind, md.valueKind)
}

// metadataOptions defines command line arguments for commands that accept a set
// of metadata entries.
type metadataOptions struct {
	metadata         metadataList
	metadataFromFile metadataList
}

func (opts *metadataOptions) registerFlags(f *flag.FlagSet) {
	opts.metadata.valueKind = "value"
	f.Var(&opts.metadata, "metadata",
		"A metadata entry (`key:value` or key(content-type):value) to attach to the package instance (can be used multiple times).")

	opts.metadataFromFile.valueKind = "path"
	f.Var(&opts.metadataFromFile, "metadata-from-file",
		"A metadata entry (`key:path` or key(content-type):path) to attach to the package instance (can be used multiple times). The path can be \"-\" to read from stdin.")
}

func (opts *metadataOptions) load(ctx context.Context) ([]cipd.Metadata, error) {
	out := make([]cipd.Metadata, 0, len(opts.metadata.entries)+len(opts.metadataFromFile.entries))

	// Convert -metadata to cipd.Metadata entries.
	for _, md := range opts.metadata.entries {
		entry := cipd.Metadata{
			Key:         md.key,
			Value:       []byte(md.value),
			ContentType: md.contentType,
		}
		// The default content type for -metadata is text/plain (since values are
		// supplied directly via the command line).
		if entry.ContentType == "" {
			entry.ContentType = "text/plain"
		}
		out = append(out, entry)
	}

	// Load -metadata-from-file entries. At most one `-metadata-from-file key:-`
	// is allowed, we have only one stdin.
	keyWithStdin := false
	for _, md := range opts.metadataFromFile.entries {
		if md.value == "-" {
			if keyWithStdin {
				return nil, makeCLIError("at most one -metadata-from-file can use \"-\" as a value")
			}
			keyWithStdin = true
		}
		entry := cipd.Metadata{
			Key:         md.key,
			ContentType: md.contentType,
		}
		var err error
		if entry.Value, err = loadMetadataFromFile(ctx, md.key, md.value); err != nil {
			return nil, err
		}
		// Guess the content type from the file extension and its body.
		if entry.ContentType == "" {
			entry.ContentType = guessMetadataContentType(md.value, entry.Value)
		}
		out = append(out, entry)
	}

	return out, nil
}

func loadMetadataFromFile(ctx context.Context, key, path string) ([]byte, error) {
	var file *os.File
	if path == "-" {
		logging.Infof(ctx, "Reading metadata %q from the stdin...", key)
		file = os.Stdin
	} else {
		logging.Infof(ctx, "Reading metadata %q from %q...", key, path)
		var err error
		file, err = os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, cipderr.BadArgument.Apply(errors.Fmt("missing metadata file: %w", err))
			}
			return nil, cipderr.IO.Apply(errors.Fmt("reading metadata file: %w", err))
		}
		defer file.Close()
	}
	// Read at most MetadataMaxLen plus one more byte to detect true EOF.
	buf := bytes.Buffer{}
	switch _, err := io.CopyN(&buf, file, common.MetadataMaxLen+1); {
	case err == nil:
		// Successfully read more than needed => the file size is too large.
		return nil, cipderr.BadArgument.Apply(errors.Fmt("the metadata value in %q is too long, should be <=%d bytes", path, common.MetadataMaxLen))
	case err != io.EOF:
		// Failed with some unexpected read error.
		return nil, cipderr.IO.Apply(errors.Fmt("error reading metadata from %q: %w", path, err))
	default:
		return buf.Bytes(), nil
	}
}

func guessMetadataContentType(path string, val []byte) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "application/json"
	case ".jwt":
		return "application/jwt"
	default:
		return http.DetectContentType(val)
	}
}
