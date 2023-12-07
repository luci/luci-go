// Copyright 2022 The LUCI Authors.
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

package quota

import (
	"context"
	"crypto/sha256"
	"encoding/ascii85"
	"time"

	"github.com/gomodule/redigo/redis"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/msgpackpb"

	"go.chromium.org/luci/server/quota/internal/quotakeys"
	"go.chromium.org/luci/server/quota/quotapb"
)

// LoadPoliciesManual ensures that the given policy config is uploaded at
// `version` for the given Application in `realm`.
//
// If a policy config already exists for `(cfg.id, realm, version)`, this
// returns immediately without checking its content. It is the application's
// responsibility to ensure that (namespace, version) always refers to the same
// `cfg` contents.
//
// Version must not contain "$" or "~".
func (a *Application) LoadPoliciesManual(ctx context.Context, realm string, version string, cfg *quotapb.PolicyConfig) (*quotapb.PolicyConfigID, error) {
	cid := &quotapb.PolicyConfigID{
		AppId:   a.id,
		Realm:   realm,
		Version: version,
	}
	return cid, a.loadPolicies(ctx, cid, cfg)
}

// LoadPoliciesAuto ensures that the given policy config is ingested with
// a content-hash version for the given Application in `realm`.
//
// If a policy config already exists for `(cfg.id, realm, version)`, this
// returns immediately without checking its content.
//
// Returns the calculated version hash.
func (a *Application) LoadPoliciesAuto(ctx context.Context, realm string, cfg *quotapb.PolicyConfig) (cid *quotapb.PolicyConfigID, err error) {
	h := sha256.New()
	err = msgpackpb.MarshalStream(h, cfg, msgpackpb.Deterministic, msgpackpb.DisallowUnknownFields)
	if err != nil {
		return nil, errors.Annotate(err, "while computing PolicyConfig hash").Err()
	}
	dat := h.Sum(nil)
	buf := make([]byte, ascii85.MaxEncodedLen(len(dat)))
	buf = buf[:ascii85.Encode(buf, dat)]

	cid = &quotapb.PolicyConfigID{
		AppId:         a.id,
		Realm:         realm,
		VersionScheme: 1,
		Version:       string(buf),
	}

	return cid, a.loadPolicies(ctx, cid, cfg)
}

const secondsInDay = uint32((time.Hour * 24) / time.Second)

func checkPolicy(p *quotapb.Policy) error {
	if p.Default > p.Limit {
		return errors.Reason("Default>Limit: %d > %d", p.Default, p.Limit).Err()
	}
	if r := p.Refill; r != nil {
		if r.Interval == 0 && r.Units < 0 {
			return errors.New("Refill: Interval=0 && Units<0")
		}
		if (secondsInDay % r.Interval) != 0 {
			return errors.Reason("Interval does not cleanly divide day: %d", r.Interval).Err()
		}
	}
	return nil
}

// loadPolicies ensures that the given policy config is loaded.
//
// If a version of the policy config already exists for this namespace+version
// pair, this returns immediately without checking its content.
//
// Returns the version string suitable for PolicyID and an error. If
// cid.VersionScheme is `Version` then this is the same value and can be ignored.
func (a *Application) loadPolicies(ctx context.Context, cid *quotapb.PolicyConfigID, cfg *quotapb.PolicyConfig) (err error) {
	if a.id == "" {
		return errors.New("invalid application")
	}

	if err = cfg.ValidateAll(); err != nil {
		return err
	}

	for i, entry := range cfg.Policies {
		if !a.resources.Has(entry.Key.ResourceType) {
			return errors.Reason("cfg.Policies[%d].Key: unknown resource type: %s", i, entry.Key.ResourceType).Err()
		}
		if err = checkPolicy(entry.Policy); err != nil {
			return errors.Annotate(err, "cfg.Policies[%d].Policy", i).Err()
		}
	}

	cfgIDKey := quotakeys.PolicyConfigID(cid)

	return withRedisConn(ctx, func(conn redis.Conn) error {
		// If this thing exists, we're done.
		exists, err := redis.Bool(conn.Do("EXISTS", cfgIDKey))
		if err != nil {
			return errors.Annotate(err, "unable to check existance of policy config %q", cfgIDKey).Err()
		}
		if exists {
			return nil
		}

		// At this point we'll call HSET; If we're racing, it's OK because the
		// application has ensured us that (namespace, versionScheme, version)
		// always maps to the same Config.
		args := make(redis.Args, 0, 1+2+(len(cfg.Policies)*2))
		args = args.Add(cfgIDKey)
		tsBytes, err := msgpackpb.Marshal(timestamppb.New(clock.Now(ctx)), msgpackpb.Deterministic)
		if err != nil {
			return errors.Annotate(err, "serializing timestamp").Err()
		}
		args = args.Add("~loaded_time", string(tsBytes))

		for i, entry := range cfg.Policies {
			polBytes, err := msgpackpb.Marshal(entry.Policy, msgpackpb.Deterministic, msgpackpb.DisallowUnknownFields)
			if err != nil {
				return errors.Annotate(err, "serializing cfg.Policies[%d]", i).Err()
			}
			args = args.Add(quotakeys.PolicyKey(entry.Key), string(polBytes))
		}

		_, err = conn.Do("HSET", args...)
		return errors.Annotate(err, "unable to load policy config").Err()
	})
}
