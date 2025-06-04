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
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/validation"
	ds "go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
)

const configFile = "tokenserver.cfg"

// ImportCAConfigsRPC implements Admin.ImportCAConfigs RPC method.
type ImportCAConfigsRPC struct {
}

// ImportCAConfigs fetches CA configs from from luci-config right now.
func (r *ImportCAConfigsRPC) ImportCAConfigs(c context.Context, _ *emptypb.Empty) (*admin.ImportedConfigs, error) {
	content, meta, err := fetchConfigFile(c, configFile)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "can't read config file - %s", err)
	}
	logging.Infof(c, "Importing tokenserver.cfg at rev %s", meta.Revision)

	// Read list of CAs.
	msg := admin.TokenServerConfig{}
	if err = prototext.Unmarshal([]byte(content), &msg); err != nil {
		return nil, status.Errorf(codes.Internal, "can't parse config file - %s", err)
	}

	seenIDs, err := LoadCAUniqueIDToCNMap(c)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "can't load unique_id map - %s", err)
	}
	if seenIDs == nil {
		seenIDs = map[int64]string{}
	}
	seenIDsDirty := false

	// There should be no duplicates.
	seenCAs := stringset.New(len(msg.GetCertificateAuthority()))
	for _, ca := range msg.GetCertificateAuthority() {
		if seenCAs.Has(ca.Cn) {
			return nil, status.Errorf(codes.Internal, "duplicate entries in the config")
		}
		seenCAs.Add(ca.Cn)
		// Check unique ID is not being reused.
		if existing, seen := seenIDs[ca.UniqueId]; seen {
			if existing != ca.Cn {
				return nil, status.Errorf(
					codes.Internal, "duplicate unique_id %d in the config: %q and %q",
					ca.UniqueId, ca.Cn, existing)
			}
		} else {
			seenIDs[ca.UniqueId] = ca.Cn
			seenIDsDirty = true
		}
	}

	// Update the mapping CA unique_id -> CA CN. Unique integer ids are used in
	// various tokens in place of a full CN name to save space. This mapping is
	// additive (all new CAs should have different IDs).
	if seenIDsDirty {
		if err := StoreCAUniqueIDToCNMap(c, seenIDs); err != nil {
			return nil, status.Errorf(codes.Internal, "can't store unique_id map - %s", err)
		}
	}

	// Add new CA datastore entries or update existing ones.
	wg := sync.WaitGroup{}
	me := errors.NewLazyMultiError(len(msg.GetCertificateAuthority()))
	for i, ca := range msg.GetCertificateAuthority() {
		wg.Add(1)
		go func(i int, ca *admin.CertificateAuthorityConfig) {
			defer wg.Done()
			content, meta, err := fetchConfigFile(c, ca.CertPath)
			if err != nil {
				logging.Errorf(c, "Failed to fetch %q: %s", ca.CertPath, err)
				me.Assign(i, err)
			} else if err := importCA(c, ca, content, meta.Revision); err != nil {
				logging.Errorf(c, "Failed to import %q: %s", ca.Cn, err)
				me.Assign(i, err)
			}
		}(i, ca)
	}
	wg.Wait()
	if err = me.Get(); err != nil {
		return nil, status.Errorf(codes.Internal, "can't import CA - %s", err)
	}

	// Find CAs that were removed from the config.
	var toRemove []string
	q := ds.NewQuery("CA").Eq("Removed", false).KeysOnly(true)
	err = ds.Run(c, q, func(k *ds.Key) {
		if !seenCAs.Has(k.StringID()) {
			toRemove = append(toRemove, k.StringID())
		}
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "datastore error - %s", err)
	}

	// Mark them as inactive in the datastore.
	wg = sync.WaitGroup{}
	me = errors.NewLazyMultiError(len(toRemove))
	for i, name := range toRemove {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			if err := removeCA(c, name, meta.Revision); err != nil {
				logging.Errorf(c, "Failed to remove %q: %s", name, err)
				me.Assign(i, err)
			}
		}(i, name)
	}
	wg.Wait()
	if err = me.Get(); err != nil {
		return nil, status.Errorf(codes.Internal, "datastore error - %s", err)
	}

	return &admin.ImportedConfigs{Revision: meta.Revision}, nil
}

// SetupConfigValidation registers the config validation rules.
func (r *ImportCAConfigsRPC) SetupConfigValidation(rules *validation.RuleSet) {
	// Validate CA config protos are well-formed.
	rules.Add("services/${appid}", configFile, func(ctx *validation.Context, configSet, path string, content []byte) error {
		cfg := &admin.TokenServerConfig{}
		if err := prototext.Unmarshal(content, cfg); err != nil {
			ctx.Errorf("not a valid TokenServerConfig proto message - %s", err)
			return nil
		}
		// These are needed to validate unique_ids are not reused. If this call
		// fails, the validation callback will be retried.
		idToCN, err := LoadCAUniqueIDToCNMap(ctx.Context)
		if err != nil {
			return transient.Tag.Apply(errors.Fmt("can't load unique_id map: %w", err))
		}
		validateCAConfigs(ctx, cfg, idToCN)
		return nil
	})

	// Validate the CA certificates are well-formed.
	rules.Add("services/${appid}", `regex:certs/.*\.pem`, func(ctx *validation.Context, configSet, path string, content []byte) error {
		if _, _, err := decodeCACert(string(content)); err != nil {
			ctx.Errorf("bad CA certificate file - %s", err)
		}
		return nil
	})
}

////////////////////////////////////////////////////////////////////////////////

// fetchConfigFile fetches a file from this services' config set.
func fetchConfigFile(c context.Context, path string) (string, *config.Meta, error) {
	logging.Infof(c, "Reading %q", path)
	c, cancel := context.WithTimeout(c, 29*time.Second) // URL fetch deadline
	defer cancel()

	var (
		content string
		meta    config.Meta
	)
	if err := cfgclient.Get(c, "services/${appid}", path, cfgclient.String(&content), &meta); err != nil {
		return "", nil, err
	}
	return content, &meta, nil
}

// decodeCACert parses x509 pem-encoded certificate and checks it is a CA cert.
//
// Returns the decoded cert, as well as its der-encoded representation.
func decodeCACert(certPem string) (cert *x509.Certificate, certDer []byte, err error) {
	certDer, err = utils.ParsePEM(certPem, "CERTIFICATE")
	if err != nil {
		return nil, nil, fmt.Errorf("bad PEM - %s", err)
	}
	switch cert, err = x509.ParseCertificate(certDer); {
	case err != nil:
		return nil, nil, fmt.Errorf("bad cert - %s", err)
	case !cert.IsCA:
		return nil, nil, fmt.Errorf("not a CA cert")
	default:
		return cert, certDer, nil
	}
}

// validateCAConfigs checks correctness of CertificateAuthorityConfig messages.
//
// Mapping from unique_id to a cert should be unique in time, so this function
// also accepts an existing mapping (fetched from the datastore), to verify
// no IDs are reused.
func validateCAConfigs(ctx *validation.Context, cfg *admin.TokenServerConfig, idToCN map[int64]string) {
	seenIDs := make(map[int64]string, len(idToCN))
	for k, v := range idToCN {
		seenIDs[k] = v
	}
	seenCAs := stringset.New(len(cfg.CertificateAuthority))
	for _, ca := range cfg.CertificateAuthority {
		ctx.Enter("CA %q", ca.Cn)
		if seenCAs.Has(ca.Cn) {
			ctx.Errorf("duplicate CA entries in the config")
		} else {
			seenCAs.Add(ca.Cn)
		}
		// Check unique ID is not being reused.
		if existing, seen := seenIDs[ca.UniqueId]; seen {
			if existing != ca.Cn {
				ctx.Errorf("unique_id %d has already been used for CA %q and can't be reused, pick another one",
					ca.UniqueId, existing)
			}
		} else {
			seenIDs[ca.UniqueId] = ca.Cn
		}
		ctx.Exit()
	}
}

// importCA imports CA definition from the config (or updates an existing one).
func importCA(c context.Context, ca *admin.CertificateAuthorityConfig, certPem string, rev string) error {
	// Deserialize the cert and check its name matches the name in the config.
	cert, certDer, err := decodeCACert(certPem)
	if cert.Subject.CommonName != ca.Cn {
		return fmt.Errorf("bad CN in the certificate, expecting %q, got %q", ca.Cn, cert.Subject.CommonName)
	}

	// Serialize the config back to proto to store it in the entity.
	cfgBlob, err := proto.Marshal(ca)
	if err != nil {
		return err
	}

	// Create or update the entity.
	return ds.RunInTransaction(c, func(c context.Context) error {
		existing := CA{CN: ca.Cn}
		err := ds.Get(c, &existing)
		if err != nil && err != ds.ErrNoSuchEntity {
			return err
		}
		// New one?
		if err == ds.ErrNoSuchEntity {
			logging.Infof(c, "Adding new CA %q", ca.Cn)
			return ds.Put(c, &CA{
				CN:         ca.Cn,
				Config:     cfgBlob,
				Cert:       certDer,
				AddedRev:   rev,
				UpdatedRev: rev,
			})
		}
		// Exists already? Check whether we should update it.
		if !existing.Removed &&
			bytes.Equal(existing.Config, cfgBlob) &&
			bytes.Equal(existing.Cert, certDer) {
			return nil
		}
		logging.Infof(c, "Updating CA %q", ca.Cn)
		existing.Config = cfgBlob
		existing.Cert = certDer
		existing.Removed = false
		existing.UpdatedRev = rev
		existing.RemovedRev = ""
		return ds.Put(c, &existing)
	}, nil)
}

// removeCA marks the CA in the datastore as removed.
func removeCA(c context.Context, name string, rev string) error {
	return ds.RunInTransaction(c, func(c context.Context) error {
		existing := CA{CN: name}
		if err := ds.Get(c, &existing); err != nil {
			return err
		}
		if existing.Removed {
			return nil
		}
		logging.Infof(c, "Removing CA %q", name)
		existing.Removed = true
		existing.RemovedRev = rev
		return ds.Put(c, &existing)
	}, nil)
}
