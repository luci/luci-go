// Copyright 2020 The LUCI Authors.
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

package eval

import (
	"context"
	"fmt"
	"sort"
	"time"

	"golang.org/x/time/rate"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
)

var clNotFound = &errors.BoolTag{Key: errors.NewTagKey("CL not found")}

// GerritChange is a CL on Gerrit.
type GerritChange struct {
	Host   string `json:"host"`
	Number int    `json:"change"`
}

// String returns the CL URL.
func (cl *GerritChange) String() string {
	return fmt.Sprintf("https://%s/c/%d", cl.Host, cl.Number)
}

// GerritPatchset is a revision of a Gerrit CL.
type GerritPatchset struct {
	Change   GerritChange `json:"cl"`
	Patchset int          `json:"patchset"`
}

// String returns the patchset URL.
func (p *GerritPatchset) String() string {
	return fmt.Sprintf("https://%s/c/%d/%d", p.Change.Host, p.Change.Number, p.Patchset)
}

type gerritClient struct {
	// getChangeRPC makes a request to Gerrit and fetches a change info.
	// Mockable.
	getChangeRPC func(ctx context.Context, host string, req *gerritpb.GetChangeRequest) (*gerritpb.ChangeInfo, error)
	limiter      *rate.Limiter
}

// ChangedFiles returns the list of files changed in the given patchset.
func (c *gerritClient) ChangedFiles(ctx context.Context, ps *GerritPatchset) ([]string, error) {
	readPatchset := func(readCache bool) (*gerritpb.RevisionInfo, error) {
		info, err := c.ReadChange(ctx, &ps.Change, readCache)
		if err != nil {
			return nil, errors.Annotate(err, "failed to read CL %s", &ps.Change).Err()
		}

		for _, r := range info.Revisions {
			if r.Number == int32(ps.Patchset) {
				return r, nil
			}
		}
		return nil, nil
	}

	rev, err := readPatchset(true)
	switch {
	case err != nil:
		return nil, err

	case rev == nil:
		// The CL is found, but the patchset is not.
		// It is possible that our cache is stale: try again without reading cache.
		switch rev, err = readPatchset(false); {
		case err != nil:
			return nil, err
		case rev == nil:
			return nil, errors.Reason("patchset %d is not found in CL %s", ps.Patchset, &ps.Change).Err()
		}
	}

	names := make([]string, 0, len(rev.Files))
	for name := range rev.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// ReadChange fetches the change.
//
// If readCache is false, ignores the cache when reading, but still caches the
// fetched change.
func (c *gerritClient) ReadChange(ctx context.Context, cl *GerritChange, readCache bool) (*gerritpb.ChangeInfo, error) {
	// TODO(crbug.com/1112125): implement caching.
	return c.fetchChange(ctx, cl)
}

func (c *gerritClient) fetchChange(ctx context.Context, cl *GerritChange) (*gerritpb.ChangeInfo, error) {
	var info *gerritpb.ChangeInfo
	err := retry.Retry(ctx, transient.Only(retry.Default), func() (err error) {
		info, err = c.fetchChangeWithQuotaErrorsRetries(ctx, cl.Host, &gerritpb.GetChangeRequest{
			Number: int64(cl.Number),
			Options: []gerritpb.QueryOption{
				gerritpb.QueryOption_ALL_REVISIONS,
				gerritpb.QueryOption_ALL_FILES,
			},
		})
		if grpcutil.IsTransientCode(status.Code(err)) {
			err = transient.Tag.Apply(err)
		}
		return
	}, retry.LogCallback(ctx, fmt.Sprintf("read CL %d", cl.Number)))

	if err != nil {
		if status.Code(err) == codes.NotFound {
			err = clNotFound.Apply(err)
		}
		return nil, err
	}

	return info, nil
}

// fetchChangeWithQuotaErrorsRetries fetches the change.
// If the request fails with quota exhaustion, retries the request in a second,
// up to 5 times.
// Does not retry other transient errors, e.g. internal errors.
func (c *gerritClient) fetchChangeWithQuotaErrorsRetries(ctx context.Context, host string, req *gerritpb.GetChangeRequest) (*gerritpb.ChangeInfo, error) {
	// Retry ResourceExchausted errors with an increased delay.
	iter := func() retry.Iterator {
		base := retry.Limited{
			Delay:   time.Second, // short-term quota resets in a second.
			Retries: 5,
		}
		return retry.NewIterator(func(ctx context.Context, err error) time.Duration {
			if status.Code(err) == codes.ResourceExhausted {
				// TODO(nodir): distinguish short- and long-term quota exhaustion.
				return base.Next(ctx, err)
			}
			return retry.Stop
		})
	}

	var ret *gerritpb.ChangeInfo
	err := retry.Retry(ctx, iter, func() (err error) {
		if err := c.limiter.Wait(ctx); err != nil {
			return err
		}
		ret, err = c.getChangeRPC(ctx, host, req)
		return
	}, nil)
	return ret, err
}
