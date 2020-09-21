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
	"net/http"
	"sort"
	"time"

	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
	"golang.org/x/time/rate"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	httpClient *http.Client
	limiter    *rate.Limiter
	clCache    cache
}

func (c *gerritClient) ChangedFiles(ctx context.Context, ps *GerritPatchset) ([]string, error) {
	readPatchset := func(useCache bool) (*gerritpb.RevisionInfo, error) {
		info, err := c.ReadCL(ctx, &ps.Change, useCache)
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
		// It is possible that our cache is stale.
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

func (c *gerritClient) ReadCL(ctx context.Context, cl *GerritChange, useCache bool) (*gerritpb.ChangeInfo, error) {
	key := fmt.Sprintf("%s-%d", cl.Host, cl.Number)

	if !useCache {
		info, err := c.fetchCL(ctx, cl)
		if err != nil {
			return nil, err
		}
		c.clCache.put(ctx, key, info)
		return info, nil
	}

	info, err := c.clCache.getOrCreate(ctx, key, func() (interface{}, error) {
		return c.fetchCL(ctx, cl)
	})
	if err != nil {
		return nil, err
	}
	return info.(*gerritpb.ChangeInfo), nil
}

func (c *gerritClient) fetchCL(ctx context.Context, cl *GerritChange) (*gerritpb.ChangeInfo, error) {
	client, err := gerrit.NewRESTClient(c.httpClient, cl.Host, true)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create a Gerrit client").Err()
	}

	var info *gerritpb.ChangeInfo
	err = retry.Retry(ctx, transient.Only(retry.Default), func() (err error) {
		info, err = c.getChangeRetryQuotaErrors(ctx, client, &gerritpb.GetChangeRequest{
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

func (c *gerritClient) getChangeRetryQuotaErrors(ctx context.Context, client gerritpb.GerritClient, req *gerritpb.GetChangeRequest) (*gerritpb.ChangeInfo, error) {
	// Retry ResourceExchausted errors with increased delay.
	iter := func() retry.Iterator {
		base := retry.Limited{
			Delay:   time.Second,
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
		ret, err = client.GetChange(ctx, req)
		return
	}, nil)
	return ret, err
}
