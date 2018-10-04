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

package distributor

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/textproto"
	"go.chromium.org/luci/dm/api/distributor"
	"go.chromium.org/luci/dm/api/service/v1"
	"go.chromium.org/luci/tumble"

	"github.com/golang/protobuf/proto"
)

var regKey = "holds a DM Distributor Registry"

// WithRegistry adds the registry to the Context.
func WithRegistry(c context.Context, r Registry) context.Context {
	if r == nil {
		panic(errors.New("you may not use WithRegistry on a nil Registry"))
	}
	return context.WithValue(c, &regKey, r)
}

// GetRegistry gets the registry from the Context. This will return nil if the
// Context does not contain a Registry.
func GetRegistry(c context.Context) Registry {
	ret, _ := c.Value(&regKey).(Registry)
	return ret
}

// FinishExecutionFn is required to eliminate a circular dependency
// between mutate <-> distributor. Essentially this just makes a new
// mutate.FinishExecution.
//
// See mutate.FinishExecutionFn for the only actual implementation of this.
type FinishExecutionFn func(c context.Context, eid *dm.Execution_ID, rslt *dm.Result) ([]tumble.Mutation, error)

// Registry holds a collection of all of the available distributor types.
type Registry interface {
	FinishExecution(c context.Context, eid *dm.Execution_ID, rslt *dm.Result) ([]tumble.Mutation, error)

	// MakeDistributor builds a distributor instance that's configured with the
	// provided config.
	//
	// The configuration for this distributor are obtained from luci-config at the
	// time an Execution is started.
	MakeDistributor(c context.Context, cfgName string) (d D, ver string, err error)
}

// FactoryMap maps nil proto.Message instances (e.g. (*MyMessage)(nil)) to the
// factory which knows how to turn a Message of that type into a distributor.
type FactoryMap map[proto.Message]Factory

// NewRegistry builds a new implementation of Registry configured to load
// configuration data from luci-config.
//
// The mapping should hold nil-ptrs of various config protos -> respective
// Factory. When loading from luci-config, when we see a given message type,
// we'll construct the distributor instance using the provided Factory.
func NewRegistry(mapping FactoryMap, fFn FinishExecutionFn) Registry {
	ret := &registry{fFn, make(map[reflect.Type]Factory, len(mapping))}
	add := func(p proto.Message, factory Factory) {
		if factory == nil {
			panic("factory is nil")
		}
		if p == nil {
			panic("proto.Message is nil")
		}

		typ := reflect.TypeOf(p)

		if _, ok := ret.data[typ]; ok {
			panic(fmt.Errorf("trying to register %q twice", typ))
		}
		ret.data[typ] = factory
	}
	for p, f := range mapping {
		add(p, f)
	}
	return ret
}

type registry struct {
	finishExecutionImpl FinishExecutionFn
	data                map[reflect.Type]Factory
}

var _ Registry = (*registry)(nil)

func (r *registry) FinishExecution(c context.Context, eid *dm.Execution_ID, rslt *dm.Result) ([]tumble.Mutation, error) {
	return r.finishExecutionImpl(c, eid, rslt)
}

func (r *registry) MakeDistributor(c context.Context, cfgName string) (d D, ver string, err error) {
	cfg, err := loadConfig(c, cfgName)
	if err != nil {
		logging.Fields{"error": err, "cfg": cfgName}.Errorf(c, "Failed to load config")
		return
	}

	ver = cfg.Version

	typ := reflect.TypeOf(cfg.Content)

	fn, ok := r.data[typ]
	if !ok {
		return nil, "", fmt.Errorf("unknown distributor type %T", cfg.Content)
	}

	d, err = fn(c, cfg)
	return
}

// loadConfig loads the named distributor configuration from luci-config,
// possibly using the in-memory or memcache version.
func loadConfig(c context.Context, cfgName string) (ret *Config, err error) {
	configSet := cfgclient.CurrentServiceConfigSet(c)

	var (
		distCfg distributor.Config
		meta    config.Meta
	)
	if err = cfgclient.Get(c, cfgclient.AsService, configSet, "distributors.cfg", textproto.Message(&distCfg), &meta); err != nil {
		return
	}

	cfgVersion := meta.Revision
	cfg, ok := distCfg.DistributorConfigs[cfgName]
	if !ok {
		err = fmt.Errorf("unknown distributor configuration: %q", cfgName)
		return
	}
	if alias := cfg.GetAlias(); alias != nil {
		cfg, ok = distCfg.DistributorConfigs[alias.OtherConfig]
		if !ok {
			err = fmt.Errorf("unknown distributor configuration: %q (via alias %q)", cfgName, alias.OtherConfig)
			return
		}
		if cfg.GetAlias() != nil {
			err = fmt.Errorf("too many levels of indirection for alias %q (points to alias %q)", cfgName, alias.OtherConfig)
			return
		}
	}

	dt := cfg.DistributorType
	if dt == nil {
		err = fmt.Errorf("blank or unrecognized distributor_type")
		return
	}
	dVal := reflect.ValueOf(dt)

	// All non-nil DistributorType's have a single field which is the actual oneof
	// value.
	implConfig := dVal.Elem().Field(0).Interface().(proto.Message)

	ret = &Config{
		info.DefaultVersionHostname(c),
		cfgName,
		cfgVersion,
		implConfig,
	}
	return
}
