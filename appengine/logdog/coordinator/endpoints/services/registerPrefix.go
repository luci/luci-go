// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"crypto/subtle"
	"errors"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/grpcutil"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

// logStreamPrefix is a placeholder for a future "register prefix" RPC call.
//
// It represents an application's intent to register a log stream prefix.
type logStreamPrefix struct {
	prefix string
	secret []byte
}

// registerPrefix registers a log stream's Prefix value.
//
// This function behaves like an RPC call, returning a gRPC error code on
// failure. This is because it will eventually be a separate RPC call when
// Butler prefix registration is implemented.
func registerPrefix(c context.Context, lsp *logStreamPrefix) (*coordinator.LogPrefix, error) {
	log.Fields{
		"prefix": lsp.prefix,
	}.Debugf(c, "Registering log prefix.")

	prefix := types.StreamName(lsp.prefix)
	if err := prefix.Validate(); err != nil {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Invalid prefix (%s): %s", lsp.prefix, err)
	}

	secret := types.PrefixSecret(lsp.secret)
	if err := secret.Validate(); err != nil {
		return nil, grpcutil.Errf(codes.InvalidArgument, "Invalid prefix secret: %s", err)
	}

	pfx := coordinator.LogPrefixFromPrefix(prefix)

	// Check for existing prefix registration (non-transactional).
	di := ds.Get(c)
	switch err := checkRegisterPrefix(ds.Get(c), lsp, pfx); err {
	case nil:
		// The prefix is already compatibly registered.
		return pfx, nil

	case ds.ErrNoSuchEntity:
		// The prefix does not exist. Proceed with transactional registration.
		break

	default:
		log.WithError(err).Errorf(c, "Failed to register prefix (non-transactional).")
		return nil, err
	}

	// The Prefix isn't registered. Register it transactionally.
	now := clock.Now(c).UTC()
	err := di.RunInTransaction(func(c context.Context) error {
		di := ds.Get(c)

		// Check if this Prefix exists (transactional).
		switch err := checkRegisterPrefix(di, lsp, pfx); err {
		case nil:
			// The prefix is already compatibly registered.
			log.Debugf(c, "Prefix is already registered.")
			return nil

		case ds.ErrNoSuchEntity:
			// The Prefix is not registered, so let's register it.
			pfx.Secret = []byte(secret)
			pfx.Created = now

			if err := di.Put(pfx); err != nil {
				log.WithError(err).Errorf(c, "Failed to register prefix.")
				return grpcutil.Internal
			}
			log.Infof(c, "The prefix was successfully registered.")
			return nil

		default:
			// Unexpected error.
			log.WithError(err).Errorf(c, "Failed to check prefix registration.")
			return err
		}
	}, nil)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to register prefix (transactional).")
		return nil, err
	}

	return pfx, nil
}

// checkRegisterPrefix is our registration logic. It will be executed once
// non-transactionally and (if registration is needed) again transactionally.
//
// If the prefix is already compatibly registered, nil will be returned. If no
// such entity exists, ds.ErrNoSuchEntity will be returned.
//
// gRPC errors returned by this method will be forwarded to the caller.
func checkRegisterPrefix(di ds.Interface, lsp *logStreamPrefix, pfx *coordinator.LogPrefix) error {
	switch err := di.Get(pfx); err {
	case nil:
		// Prefix registered, does it match? If so, this is an idempotent operation.
		if err := matchesLogPrefix(lsp, pfx); err != nil {
			return grpcutil.Errf(codes.AlreadyExists, "Log prefix is already registered: %v", err)
		}

		return nil

	case ds.ErrNoSuchEntity:
		return err

	default:
		return grpcutil.Internal
	}
}

func matchesLogPrefix(lsp *logStreamPrefix, pfx *coordinator.LogPrefix) error {
	if lsp.prefix != pfx.Prefix {
		return errors.New("prefixes do not match")
	}
	if subtle.ConstantTimeCompare(lsp.secret, pfx.Secret) != 1 {
		return errors.New("secrets do not match")
	}
	return nil
}
