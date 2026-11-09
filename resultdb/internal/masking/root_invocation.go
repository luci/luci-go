// Copyright 2025 The LUCI Authors.
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

package masking

import (
	"fmt"
	"regexp"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// RootInvocation constructs a *pb.RootInvocation from the given fields,
// applying masking appropriate to the access level.
// The producer resource URL is computed based on the service configuration.
func RootInvocation(r *rootinvocations.RootInvocationRow, accessLevel permissions.AccessLevel, cfg *config.CompiledServiceConfig) *pb.RootInvocation {
	if accessLevel == permissions.NoAccess {
		return nil
	}

	result := &pb.RootInvocation{
		Name:                 r.RootInvocationID.Name(),
		RootInvocationId:     string(r.RootInvocationID),
		FinalizationState:    r.FinalizationState,
		State:                r.State,
		Realm:                r.Realm,
		CreateTime:           pbutil.MustTimestampProto(r.CreateTime),
		Creator:              r.CreatedBy,
		LastUpdated:          pbutil.MustTimestampProto(r.LastUpdated),
		Sources:              r.Sources,
		BaselineId:           r.BaselineID,
		Etag:                 RootInvocationEtag(r, accessLevel),
		StreamingExportState: r.StreamingExportState,
		IsMasked:             true,
	}
	if r.ProducerResource != nil {
		// Clone to avoid modifying the original row.
		result.ProducerResource = proto.Clone(r.ProducerResource).(*pb.ProducerResource)
		result.ProducerResource.Url = producerResourceURL(result.ProducerResource, cfg)
	}
	if r.FinalizeStartTime.Valid {
		result.FinalizeStartTime = pbutil.MustTimestampProto(r.FinalizeStartTime.Time)
	}
	if r.FinalizeTime.Valid {
		result.FinalizeTime = pbutil.MustTimestampProto(r.FinalizeTime.Time)
	}

	if accessLevel == permissions.FullAccess {
		result.SummaryMarkdown = r.SummaryMarkdown
		result.Definition = r.Definition
		result.Tags = r.Tags
		result.Properties = r.Properties
		result.IsMasked = false

		if r.PrimaryBuild != nil {
			result.PrimaryBuild = buildDescriptorWithURL(r.PrimaryBuild, cfg)
		}
		if r.ExtraBuilds != nil {
			// Clone to avoid modifying the original slice.
			extraBuildsWithURLs := make([]*pb.BuildDescriptor, len(r.ExtraBuilds))
			for i, b := range r.ExtraBuilds {
				extraBuildsWithURLs[i] = buildDescriptorWithURL(b, cfg)
			}
			result.ExtraBuilds = extraBuildsWithURLs
		}
	} else {
		// Include a truncated version of the summary.
		result.SummaryMarkdown = pbutil.TruncateString(r.SummaryMarkdown, limitedSummaryLength)
		// Include a masked version of the root invocation definition.
		if r.Definition != nil {
			def := proto.Clone(r.Definition).(*pb.RootInvocationDefinition)
			def.Properties = nil
			result.Definition = def
		}
	}
	return result
}

// RootInvocationMetadata constructs a *pb.RootInvocationMetadata from the given fields.
// The producer resource URL is computed based on the service configuration.
func RootInvocationMetadata(r *rootinvocations.RootInvocationRow, cfg *config.CompiledServiceConfig) *pb.RootInvocationMetadata {
	rootInvocation := RootInvocation(r, permissions.FullAccess, cfg)
	return &pb.RootInvocationMetadata{
		Name:             rootInvocation.Name,
		RootInvocationId: rootInvocation.RootInvocationId,
		Realm:            rootInvocation.Realm,
		CreateTime:       rootInvocation.CreateTime,
		Definition:       rootInvocation.Definition,
		Sources:          rootInvocation.Sources,
		ProducerResource: rootInvocation.ProducerResource,
		PrimaryBuild:     rootInvocation.PrimaryBuild,
		ExtraBuilds:      rootInvocation.ExtraBuilds,
	}
}

// buildDescriptorWithURL returns a new BuildDescriptor with the URL field set.
func buildDescriptorWithURL(b *pb.BuildDescriptor, cfg *config.CompiledServiceConfig) *pb.BuildDescriptor {
	// Clone to avoid modifying the original proto.
	result := proto.Clone(b).(*pb.BuildDescriptor)
	if ab := result.GetAndroidBuild(); ab != nil {
		result.Url = cfg.AndroidBuild.GenerateBuildDescriptorURL(ab)
	}
	return result
}

// RootInvocationEtag returns the HTTP ETag for the given root invocation.
func RootInvocationEtag(r *rootinvocations.RootInvocationRow, accessLevel permissions.AccessLevel) string {
	accessLevelFilter := ""
	switch accessLevel {
	case permissions.LimitedAccess:
		accessLevelFilter = "+l/"
	case permissions.FullAccess:
		// Default case, keep accessLevelFilter empty.
	default:
		panic("invariant violated: invalid access level for etag generation")
	}

	// The ETag must be a function of the resource representation according to (AIP-154).
	// The representation of a root invocation depends on the access level, so we
	// include it in the ETag.
	return fmt.Sprintf(`W/"%s%s"`, accessLevelFilter, r.LastUpdated.UTC().Format(time.RFC3339Nano))
}

// rootInvocationEtagRegexp extracts the root invocation's last updated timestamp from a root invocation ETag.
var rootInvocationEtagRegexp = regexp.MustCompile(`^W/"(?:\+l/)?(.*)"$`)

// ParseRootInvocationEtag validate the etag and returns the embedded lastUpdated time.
func ParseRootInvocationEtag(etag string) (lastUpdated string, err error) {
	m := rootInvocationEtagRegexp.FindStringSubmatch(etag)
	if len(m) < 2 {
		return "", errors.Fmt("malformated etag")
	}
	return m[1], nil
}

// IsRootInvocationEtagMatch determines if the Etag is consistent with the specified
// root invocation version.
func IsRootInvocationEtagMatch(r *rootinvocations.RootInvocationRow, etag string) (bool, error) {
	lastUpdated, err := ParseRootInvocationEtag(etag)
	if err != nil {
		return false, err
	}
	return lastUpdated == r.LastUpdated.UTC().Format(time.RFC3339Nano), nil
}
