// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package model defines datastore models used by the token server.
package model

import (
	"github.com/golang/protobuf/proto"

	"github.com/luci/luci-go/common/api/tokenserver/v1"
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

	AddedRev   string `gae:",noindex"` // config rev when this CA appeared
	UpdatedRev string `gae:",noindex"` // config rev when this CA was updated
	RemovedRev string `gae:",noindex"` // config rev when it was removed
}

// ParseConfig parses proto message stored in Config.
func (c *CA) ParseConfig() (*tokenserver.CertificateAuthorityConfig, error) {
	msg := &tokenserver.CertificateAuthorityConfig{}
	if err := proto.Unmarshal(c.Config, msg); err != nil {
		return nil, err
	}
	return msg, nil
}
