// Copyright 2017 The LUCI Authors.
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

package buildbot

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/data/stringset"
	miloProto "go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/milo/api/buildbot"
)

// Resolve LogDog annotation stream for this build.
func getLogDogAnnotationAddr(c context.Context, build *buildbot.Build) (*types.StreamAddr, error) {
	if v, ok := build.PropertyValue("log_location").(string); ok && v != "" {
		return types.ParseURL(v)
	}
	return nil, grpcutil.Errf(codes.NotFound, "annotation stream not found")
}

// mergeBuildInfoIntoAnnotation merges BuildBot-specific build information into
// a LogDog annotation protobuf.
//
// This consists of augmenting the Step's properties with BuildBot's properties,
// favoring the Step's version of the properties if there are two with the same
// name.
func mergeBuildIntoAnnotation(c context.Context, step *miloProto.Step, build *buildbot.Build) error {
	allProps := stringset.New(len(step.Property) + len(build.Properties))
	for _, prop := range step.Property {
		allProps.Add(prop.Name)
	}
	for _, prop := range build.Properties {
		// Annotation protobuf overrides BuildBot properties.
		if allProps.Has(prop.Name) {
			continue
		}
		allProps.Add(prop.Name)

		step.Property = append(step.Property, &miloProto.Step_Property{
			Name:  prop.Name,
			Value: fmt.Sprintf("%v", prop.Value),
		})
	}

	return nil
}
