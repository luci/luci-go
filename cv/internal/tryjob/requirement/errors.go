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

const canonicalDirectiveFormat = "project/bucket:builder1,builder2;project2/bucket:builder3"

// invalidTryjobDirectives is a computation failure where the value of the
// Tryjob directives provided in the footer is malformed.
type invalidTryjobDirectives struct {
	value string
}

func (itd *invalidTryjobDirectives) Reason() string {
	return fmt.Sprintf("The tryjob directive %q is invalid. Canonical format is %s", itd.value, canonicalDirectiveFormat)
}

// unauthorizedIncludedTryjob is a computation failure where a user is not
// allowed to trigger a certain builder.
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

// incompatibleTryjobOptions is a computation failure where multiple Tryjob
// options have been provided but they are not compatible with each other.
type incompatibleTryjobOptions struct {
	hasIncludedTryjobs   bool
	hasOverriddenTryjobs bool
}

func (ito *incompatibleTryjobOptions) Reason() string {
	switch {
	case ito.hasIncludedTryjobs && ito.hasOverriddenTryjobs:
		return "Both `Cq-Include-Trybots` and `Override-Tryjobs-For-Automation` are specified. Only one is allowed."
	default:
		panic(fmt.Errorf("impossible"))
	}
}
