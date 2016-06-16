// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"time"

	"github.com/luci/gae/service/datastore"
	log "github.com/luci/luci-go/common/logging"

	"golang.org/x/net/context"
)

// getMasterJSON fetches the latest known buildbot master data and returns
// the buildbotMaster struct (if found), whether or not it is internal,
// the last modified time, and an error if not found.
func getMasterJSON(c context.Context, name string) (
	master *buildbotMaster, internal bool, t time.Time, err error) {
	master = &buildbotMaster{}
	entry := buildbotMasterEntry{Name: name}
	ds := datastore.Get(c)
	err = ds.Get(&entry)
	internal = entry.Internal
	t = entry.Modified
	if err != nil {
		return
	}
	reader, err := gzip.NewReader(bytes.NewReader(entry.Data))
	if err != nil {
		log.WithError(err).Errorf(
			c, "Encountered error while fetching entry for %s:\n%s", name, err)
		return
	}
	defer reader.Close()
	d := json.NewDecoder(reader)
	if err = d.Decode(master); err != nil {
		log.WithError(err).Errorf(
			c, "Encountered error while fetching entry for %s:\n%s", name, err)
	}
	return
}
