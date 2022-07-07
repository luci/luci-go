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

package requirement

import (
	"fmt"
	"strings"
)

const canonicalIncludeDirectiveFormat = "project/bucket:builder1,builder2;project2/bucket:builder3"

// invalidCQIncludedTryjobs is a computation failure where the value of the
// Cq-Include-Trybots footer is malformed.
type invalidCQIncludedTryjobs struct {
	value string
}

func (icqit *invalidCQIncludedTryjobs) Reason() string {
	return fmt.Sprintf("The included tryjob spec %q is invalid. Canonical format is %s", icqit.value, canonicalIncludeDirectiveFormat)
}

// unauthorizedIncludedTryjob is a computation failure where a user is not allowed to
// trigger a certain builder.
type unauthorizedIncludedTryjob struct {
	Users   []string
	Builder string
}

func (uit *unauthorizedIncludedTryjob) Reason() string {
	switch len(uit.Users) {
	case 0:
		panic(fmt.Errorf("impossible, at least one unauthorized user is needed in this failure"))
	case 1:
		return fmt.Sprintf("user %s is not allowed to trigger %q", uit.Users[0], uit.Builder)
	default:
		return fmt.Sprintf("the following users are not allowed to trigger %q:\n - %s", uit.Builder, strings.Join(uit.Users, "\n - "))
	}
}

// buildersNotDefined represents a computation failure where builders explicitly
// included in the Run are not known to CV.
type buildersNotDefined struct {
	Builders []string
}

func (bnd *buildersNotDefined) Reason() string {
	switch len(bnd.Builders) {
	case 0:
		panic(fmt.Errorf("impossible: at least one undefined builder is needed in this failure"))
	case 1:
		return fmt.Sprintf("builder %q is included but not defined in the LUCI project", bnd.Builders[0])
	default:
		return fmt.Sprintf("the following builders are included but not defined in the LUCI project:\n - %s", strings.Join(bnd.Builders, "\n - "))
	}
}

// buildersNotDirectlyIncludable represents a computation failure where builders
// explicitly included in the Run are to be triggered by some other builder and
// not by CV directly.
type buildersNotDirectlyIncludable struct {
	Builders []string
}

func (bni *buildersNotDirectlyIncludable) Reason() string {
	switch len(bni.Builders) {
	case 0:
		panic(fmt.Errorf("impossible: at least one unincludable builder is needed in this failure"))
	case 1:
		return fmt.Sprintf("builder %q is included but cannot be triggered directly by CV", bni.Builders[0])
	default:
		return fmt.Sprintf("the following builders are included but cannot be triggered directly by CV:\n - %s", strings.Join(bni.Builders, "\n - "))
	}
}
