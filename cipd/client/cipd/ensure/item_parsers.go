// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package ensure

import (
	"fmt"
	"net/url"

	"github.com/luci/luci-go/cipd/client/cipd/common"
)

// an itemParser should parse the value from `val`, and update s or
// f accordingly, returning an error if needed.
type itemParser func(s *itemParserState, f *File, val string) error

// itemParserState is the state object shared between the item parsers and the
// main ParseFile implementation.
type itemParserState struct {
	curSubdir string
}

func subdirParser(s *itemParserState, _ *File, val string) error {
	if err := common.ValidateSubdir(val); err != nil {
		return err
	}
	s.curSubdir = val
	return nil
}

func serviceURLParser(_ *itemParserState, f *File, val string) error {
	if f.ServiceURL != "" {
		return fmt.Errorf("$ServiceURL may only be set once per file")
	}
	if _, err := url.Parse(val); err != nil {
		return fmt.Errorf("expecting '$ServiceURL <url>' but url is invalid: %s", err)
	}
	f.ServiceURL = val
	return nil
}

// itemParsers is the main way that the ensure file format is extended. If you
// need to add a new setting or directive, please add an appropriate function
// above and then add it to this map.
var itemParsers = map[string]itemParser{
	"@subdir":     subdirParser,
	"$serviceurl": serviceURLParser,
}
