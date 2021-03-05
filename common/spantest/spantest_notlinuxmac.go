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

// +build !darwin
// +build !linux

// Package spantest implements setups to run Spanner tests using the Cloud Spanner Emulator.
package spantest

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"

	"cloud.google.com/go/spanner"
	spandb "cloud.google.com/go/spanner/admin/database/apiv1"
	spanins "cloud.google.com/go/spanner/admin/instance/apiv1"
	"google.golang.org/api/option"
	dbpb "google.golang.org/genproto/googleapis/spanner/admin/database/v1"
	inspb "google.golang.org/genproto/googleapis/spanner/admin/instance/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/common/system/filesystem"
	"go.chromium.org/luci/common/system/port"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

// StartEmulator starts a Cloud Spanner Emulator instance.
func StartEmulator(ctx context.Context) (*Emulator, error) {
	return nil, fmt.Errorf("can only run spanner tests on linux or mac.")
}

// Stop kills the emulator process and removes the temporary gcloud config directory.
func (e *Emulator) Stop() error {
	return fmt.Errorf("can only run spanner tests on linux or mac.")
}


