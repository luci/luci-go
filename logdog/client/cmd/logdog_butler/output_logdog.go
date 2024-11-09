// Copyright 2015 The LUCI Authors.
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

package main

import (
	"fmt"
	"time"

	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/multiflag"

	"go.chromium.org/luci/logdog/client/butler/output"
	out "go.chromium.org/luci/logdog/client/butler/output/logdog"
)

func init() {
	registerOutputFactory(new(logdogOutputFactory))
}

// logdogOutputFactory for publishing logs using a LogDog Coordinator host.
type logdogOutputFactory struct {
	service          string
	prefixExpiration clockflag.Duration
}

var _ outputFactory = (*logdogOutputFactory)(nil)

func (f *logdogOutputFactory) option() multiflag.Option {
	opt := newOutputOption("logdog", "Output to a LogDog Coordinator instance.", f)

	flags := opt.Flags()
	flags.StringVar(&f.service, "service", "",
		"Optional service within <host> to use. Will be referenced as <service>-dot-<host>.")
	flags.Var(&f.prefixExpiration, "prefix-expiration",
		"Amount of time after registration that the prefix will be active. If omitted, the service "+
			"default will be used. This should exceed the expected lifetime of the job by a fair margin.")

	return opt
}

func (f *logdogOutputFactory) configOutput(a *application) (output.Output, error) {
	auth, err := a.authenticator(a)
	if err != nil {
		return nil, errors.Annotate(err, "failed to instantiate authenticator").Err()
	}

	host := a.coordinatorHost
	if host == "" {
		return nil, errors.New("logdog output requires a Coordinator host (-coordinator-host)")
	}
	if f.service != "" {
		host = fmt.Sprintf("%s-dot-%s", f.service, host)
	}

	cfg := out.Config{
		Auth:             out.LegacyAuth(auth),
		Host:             host,
		Project:          a.project,
		Prefix:           a.prefix,
		PrefixExpiration: time.Duration(f.prefixExpiration),
		SourceInfo: []string{
			"LogDog Butler",
		},
		PublishContext: a.ncCtx,
		RPCTimeout:     30 * time.Second,
	}
	return cfg.Register(a)
}

func (f *logdogOutputFactory) scopes() []string { return out.Scopes() }
