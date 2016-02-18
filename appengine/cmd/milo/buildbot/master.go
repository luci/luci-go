// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package buildbot

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path"
	"strings"

	"golang.org/x/net/context"
)

// getMasterJSON fetches the master json from buildbot.  This only works
// for public waterfalls because appengine does not have access to the internal
// ones.
func getMasterJSON(c context.Context, mastername string) ([]byte, error) {
	// TODO(hinoka): Check local cache, CBE, etc.
	if strings.HasPrefix(mastername, "debug:") {
		filename := path.Join("testdata", fmt.Sprintf("master:%s", mastername[6:]))
		b, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		return b, nil
	}

	URL := fmt.Sprintf("https://build.chromium.org/p/%s/json", mastername)
	return getURL(c, URL)
}

// getMaster fetches the master json and returns the result as a unmarshaled
// buildbotMaster struct.
func getMaster(c context.Context, mastername string) (*buildbotMaster, error) {
	b, err := getMasterJSON(c, mastername)
	if err != nil {
		return nil, err
	}
	result := &buildbotMaster{}
	if err = json.Unmarshal(b, result); err != nil {
		return nil, err
	}
	return result, nil
}
