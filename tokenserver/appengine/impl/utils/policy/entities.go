// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package policy

import (
	"golang.org/x/net/context"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/retry/transient"
)

// importedPolicyHeader is an entity that holds metadata about a cached policy.
//
// Most crucially, it stores a digest of serialized policy configs. It is used
// by Policy.Queryable() to quickly detect that no changes to the config have
// been made.
//
// The actual serialized configs (that are much heavier than metadata) are
// stored in a separate child entity, fetched only when they are really needed.
type importedPolicyHeader struct {
	_kind string `gae:"$kind,ImportedPolicyHeader"`

	Name     string `gae:"$id"`      // name of the corresponding policy
	Revision string `gae:",noindex"` // last ingested config revision
	SHA256   string `gae:",noindex"` // SHA256 (hex) of the serialized ConfigBundle
}

// importedPolicyBody is a fat entity that contains serialized policy configs.
//
// It's a single child of ImportedPolicyHeader.
type importedPolicyBody struct {
	_kind string `gae:"$kind,ImportedPolicyBody"`
	_id   string `gae:"$id,1"`

	Parent   *datastore.Key `gae:"$parent"`  // key of ImportedPolicyHeader
	Revision string         `gae:",noindex"` // last ingested config revision
	SHA256   string         `gae:",noindex"` // SHA256 (hex) of the Data below
	Data     []byte         `gae:",noindex"` // serialized ConfigBundle
}

// updateImportedPolicy replaces the currently stored policy.
//
// It transactionally updates both importedPolicyHeader and importedPolicyBody.
func updateImportedPolicy(c context.Context, name, rev, sha256 string, serialized []byte) error {
	header := &importedPolicyHeader{
		Name:     name,
		Revision: rev,
		SHA256:   sha256,
	}
	body := &importedPolicyBody{
		Parent:   datastore.KeyForObj(c, header),
		Revision: rev,
		SHA256:   sha256,
		Data:     serialized,
	}
	return transient.Tag.Apply(datastore.RunInTransaction(c, func(c context.Context) error {
		return datastore.Put(c, header, body)
	}, nil))
}

// getImportedPolicyHeader loads importedPolicyHeader entity from the datastore.
//
// Returns (nil, nil) if there's no such entity.
func getImportedPolicyHeader(c context.Context, name string) (*importedPolicyHeader, error) {
	e := &importedPolicyHeader{Name: name}
	switch err := datastore.Get(c, e); {
	case err == datastore.ErrNoSuchEntity:
		return nil, nil
	case err != nil:
		return nil, transient.Tag.Apply(err)
	}
	return e, nil
}

// getImportedPolicyBody loads importedPolicyBody entity from the datastore.
//
// Returns (nil, nil) if there's no such entity.
func getImportedPolicyBody(c context.Context, name string) (*importedPolicyBody, error) {
	e := &importedPolicyBody{
		Parent: datastore.KeyForObj(c, &importedPolicyHeader{Name: name}),
	}
	switch err := datastore.Get(c, e); {
	case err == datastore.ErrNoSuchEntity:
		return nil, nil
	case err != nil:
		return nil, transient.Tag.Apply(err)
	}
	return e, nil
}
