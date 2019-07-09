// Copyright 2019 The LUCI Authors.
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

package dispatcher

import (
	"go.chromium.org/luci/common/errors"
)

// Normalize validates that Options is well formed and populates defaults which
// are missing.
func (o *Options) Normalize() error {
	if o.SendFn == nil {
		return errors.New("SendFn is required: got nil")
	}

	if o.ErrorFn == nil {
		o.ErrorFn = Defaults.ErrorFn
	}

	if o.MaxSenders == 0 {
		o.MaxSenders = Defaults.MaxSenders
	} else if o.MaxSenders < 1 {
		return errors.Reason("MaxSenders must be > 0: got %d", o.MaxSenders).Err()
	}

	if o.MaxQPS == 0 {
		o.MaxQPS = Defaults.MaxQPS
	} else if o.MaxQPS < 0 {
		return errors.Reason("MaxQPS must be > 0: got %f", o.MaxQPS).Err()
	}

	if err := o.Buffer.Normalize(); err != nil {
		return errors.Annotate(err, "normalizing Buffer").Err()
	}

	return nil
}
