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

package retry

import (
	"context"
	"time"
)

// defaultIterator defines a template for the default retry parameters that
// should be used throughout the program.
var defaultIteratorTemplate = ExponentialBackoff{
	Limited: Limited{
		Delay:   200 * time.Millisecond,
		Retries: 10,
	},
	MaxDelay:   10 * time.Second,
	Multiplier: 2,
}

// Default is a Factory that returns a new instance of the default iterator
// configuration.
func Default() Iterator {
	it := defaultIteratorTemplate
	return &it
}

type noneItTemplate struct{}

func (noneItTemplate) Next(context.Context, error) time.Duration { return Stop }

// None is a Factory that returns an Iterator that explicitly calls Stop after
// the first try. This is helpful to pass to libraries which use retry.Default
// if given nil, but where you don't want any retries at all (e.g. tests).
func None() Iterator {
	return noneItTemplate{}
}
