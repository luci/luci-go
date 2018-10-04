// Copyright 2016 The LUCI Authors.
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

package certconfig

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/gob"
	"time"

	"github.com/golang/protobuf/proto"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
)

// CA defines one trusted Certificate Authority (imported from config).
//
// Entity key is CA Common Name (that must match what's is in the certificate).
// Certificate issuer (and the certificate signature) is ignored. Usually, the
// certificates here will be self-signed.
//
// Removed CAs are kept in the datastore, but not actively used.
type CA struct {
	// CN is CA's Common Name.
	CN string `gae:"$id"`

	// Config is serialized CertificateAuthorityConfig proto message.
	Config []byte `gae:",noindex"`

	// Cert is a certificate of this CA (in der encoding).
	//
	// It is read from luci-config from path specified in the config.
	Cert []byte `gae:",noindex"`

	// Removed is true if this CA has been removed from the config.
	Removed bool

	// Ready is false before this CA's CRL is fetched for the first time.
	Ready bool

	AddedRev   string `gae:",noindex"` // config rev when this CA appeared
	UpdatedRev string `gae:",noindex"` // config rev when this CA was updated
	RemovedRev string `gae:",noindex"` // config rev when it was removed

	// ParsedConfig is parsed Config.
	//
	// Populated if CA is fetched through CertChecker.
	ParsedConfig *admin.CertificateAuthorityConfig `gae:"-"`

	// ParsedCert is parsed Cert.
	//
	// Populated if CA is fetched through CertChecker.
	ParsedCert *x509.Certificate `gae:"-"`
}

// ParseConfig parses proto message stored in Config.
func (c *CA) ParseConfig() (*admin.CertificateAuthorityConfig, error) {
	msg := &admin.CertificateAuthorityConfig{}
	if err := proto.Unmarshal(c.Config, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// ListCAs returns names of all currently active CAs, in no particular order.
func ListCAs(c context.Context) ([]string, error) {
	keys := []*ds.Key{}
	q := ds.NewQuery("CA").Eq("Removed", false).KeysOnly(true)
	if err := ds.GetAll(c, q, &keys); err != nil {
		return nil, transient.Tag.Apply(err)
	}
	names := make([]string, len(keys))
	for i, key := range keys {
		names[i] = key.StringID()
	}
	return names, nil
}

// CAUniqueIDToCNMap is a singleton entity that stores a mapping between CA's
// unique_id (specified in config) and its Common Name.
//
// It's loaded in memory in full and kept cached there (for 1 min).
// See GetCAByUniqueID below.
type CAUniqueIDToCNMap struct {
	_id int64 `gae:"$id,1"`

	GobEncodedMap []byte `gae:",noindex"` // gob-encoded map[int64]string
}

// StoreCAUniqueIDToCNMap overwrites CAUniqueIDToCNMap with new content.
func StoreCAUniqueIDToCNMap(c context.Context, mapping map[int64]string) error {
	buf := bytes.Buffer{}
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(mapping); err != nil {
		return err
	}
	// Note that in practice 'mapping' is usually very small, so we are not
	// concerned about 1MB entity size limit.
	return transient.Tag.Apply(ds.Put(c, &CAUniqueIDToCNMap{
		GobEncodedMap: buf.Bytes(),
	}))
}

// LoadCAUniqueIDToCNMap loads CAUniqueIDToCNMap from the datastore.
func LoadCAUniqueIDToCNMap(c context.Context) (map[int64]string, error) {
	ent := CAUniqueIDToCNMap{}
	switch err := ds.Get(c, &ent); {
	case err == ds.ErrNoSuchEntity:
		return nil, nil
	case err != nil:
		return nil, transient.Tag.Apply(err)
	}
	dec := gob.NewDecoder(bytes.NewReader(ent.GobEncodedMap))
	out := map[int64]string{}
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// holds cached result of LoadCAUniqueIDToCNMap().
var mappingCache = caching.RegisterCacheSlot()

// GetCAByUniqueID returns CN name that corresponds to given unique ID.
//
// It uses cached CAUniqueIDToCNMap for lookups. Returns empty string if there's
// no such CA.
func GetCAByUniqueID(c context.Context, id int64) (string, error) {
	cached, err := mappingCache.Fetch(c, func(interface{}) (interface{}, time.Duration, error) {
		val, err := LoadCAUniqueIDToCNMap(c)
		return val, time.Minute, err
	})
	if err != nil {
		return "", err
	}
	mapping := cached.(map[int64]string)
	return mapping[id], nil
}
