// Copyright 2021 The LUCI Authors.
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

// +build !darwin
// +build !linux

package spantest

import (
	"context"
	"fmt"
)

// StartEmulator starts a Cloud Spanner Emulator instance.
func StartEmulator(ctx context.Context) (*Emulator, error) {
	return nil, fmt.Errorf("can only run spanner tests on linux or mac.")
}

// Stop kills the emulator process and removes the temporary gcloud config directory.
func (e *Emulator) Stop() error {
	return fmt.Errorf("can only run spanner tests on linux or mac.")
}
