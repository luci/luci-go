// Copyright 2018 The LUCI Authors.
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

package cipd

import (
	"encoding/json"
	"fmt"
	"time"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/common"
)

// Helper structs and functions for working with JSON representation of CIPD
// domain objects.
//
// See also acl.go for ACL-related structs and action_plan.go structs related
// to EnsurePackages call.
//
// These structs largely define public API of 'cipd ... -json-output ...'.

// UnixTime is time.Time that serializes to integer unix timestamp in JSON
// (represented as a number of seconds since January 1, 1970 UTC).
type UnixTime time.Time

// String is needed to be able to print UnixTime.
func (t UnixTime) String() string {
	return time.Time(t).String()
}

// Before is used to compare UnixTime objects.
func (t UnixTime) Before(t2 UnixTime) bool {
	return time.Time(t).Before(time.Time(t2))
}

// IsZero reports whether t represents the zero time instant.
func (t UnixTime) IsZero() bool {
	return time.Time(t).IsZero()
}

// MarshalJSON is used by JSON encoder.
func (t UnixTime) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("0"), nil
	}
	return []byte(fmt.Sprintf("%d", time.Time(t).Unix())), nil
}

// JSONError is wrapper around Error that serializes it as string.
type JSONError struct {
	error
}

// MarshalJSON is used by JSON encoder.
func (e JSONError) MarshalJSON() ([]byte, error) {
	if e.error == nil {
		return []byte("null"), nil
	}
	return json.Marshal(e.Error())
}

// InstanceInfo is information about single package instance.
type InstanceInfo struct {
	// Pin identifies package instance.
	Pin common.Pin `json:"pin"`
	// RegisteredBy is identity of whoever uploaded this instance.
	RegisteredBy string `json:"registered_by"`
	// RegisteredTs is when the instance was registered.
	RegisteredTs UnixTime `json:"registered_ts"`
}

// TagInfo is returned by DescribeInstance.
type TagInfo struct {
	// Tag is actual tag name ("key:value" pair).
	Tag string `json:"tag"`
	// RegisteredBy is identity of whoever attached this tag.
	RegisteredBy string `json:"registered_by"`
	// RegisteredTs is when the tag was registered.
	RegisteredTs UnixTime `json:"registered_ts"`
}

// RefInfo is returned by DescribeInstance and FetchPackageRefs.
type RefInfo struct {
	// Ref is the ref name.
	Ref string `json:"ref"`
	// InstanceID is ID of a package instance the ref points to.
	InstanceID string `json:"instance_id"`
	// ModifiedBy is identity of whoever modified this ref last time.
	ModifiedBy string `json:"modified_by"`
	// ModifiedTs is when the ref was modified last time.
	ModifiedTs UnixTime `json:"modified_ts"`
}

// Metadata is a metadata entry that can be attached to an instance.
type Metadata struct {
	// Key is a lowercase string matching [a-z0-9_\-]{1,400}.
	Key string `json:"key"`
	// Value is an arbitrary byte blob smaller than 512 Kb.
	Value []byte `json:"value,omitempty"`
	// Optional MIME content type of the metadata value, primarily for UI.
	ContentType string `json:"content_type,omitempty"`
}

// MetadataInfo describes an already attached metadata entry.
type MetadataInfo struct {
	// Fingerprint identifies this particular metadata entry.
	//
	// It is derived from the key+value via common.InstanceMetadataFingerprint.
	Fingerprint string `json:"fingerprint"`

	// Key is a lowercase string matching [a-z0-9_\-]{1,400}.
	Key string `json:"key"`
	// Value is an arbitrary byte blob smaller than 512 Kb.
	Value []byte `json:"value,omitempty"`
	// Optional MIME content type of the metadata value, primarily for UI.
	ContentType string `json:"content_type,omitempty"`

	// AttachedBy is identity of whoever attached this metadata.
	AttachedBy string `json:"attached_by"`
	// AttachedTs is when the metadata was attached.
	AttachedTs UnixTime `json:"attached_ts"`
}

// InstanceDescription contains extended information about an instance as
// returned by DescribeInstance.
type InstanceDescription struct {
	InstanceInfo

	// Refs is a list of refs pointing to the instance, sorted by modification
	// timestamp (newest first)
	//
	// Present only if DescribeRefs in DescribeInstanceOpts is true.
	Refs []RefInfo `json:"refs,omitempty"`

	// Tags is a list of tags attached to the instance, sorted by tag key and
	// creation timestamp (newest first).
	//
	// Present only if DescribeTags in DescribeInstanceOpts is true.
	Tags []TagInfo `json:"tags,omitempty"`

	// Metadata is a list of metadata attached to the instance.
	//
	// Present only if DescribeMetadata in DescribeInstanceOpts is true.
	Metadata []MetadataInfo `json:"metadata,omitempty"`
}

// ClientDescription contains extended information about a CIPD client binary
// at some version for some platform, as returned by DescribeClient.
type ClientDescription struct {
	InstanceInfo

	// Size of the client binary file in bytes.
	Size int64 `json:"size"`

	// SignedURL is URL of the client binary.
	SignedURL string `json:"signed_url"`

	// Digest is the client binary digest using the best hash algo understood by
	// the current process.
	//
	// May potentially be nil if the current process doesn't understand any of
	// the algos, but this is an extreme situation and it is OK to panic in this
	// case. At very least all client binaries have SHA1 digests, and it should
	// be understood by all clients.
	Digest *caspb.ObjectRef `json:"digest"`

	// AlternativeDigests is a list of digest calculated using hash algos other
	// than the one used by Digest.
	//
	// This may include both old hash algos (obsoleted by the algo in Digest), as
	// well as a new ones, not supported by the current process.
	//
	// Not all client versions have all digests calculated. Older versions have
	// only SHA1 digest, which means for them Digest will be SHA1 and
	// AlternativeDigests list will be empty.
	AlternativeDigests []*caspb.ObjectRef `json:"alternative_digests"`
}

////////////////////////////////////////////////////////////////////////////////
// Converters from proto API to JSON output structs.

func apiInstanceToInfo(inst *repopb.Instance) InstanceInfo {
	var t time.Time
	if inst.RegisteredTs.IsValid() {
		t = inst.RegisteredTs.AsTime()
	}
	return InstanceInfo{
		Pin: common.Pin{
			PackageName: inst.Package,
			InstanceID:  common.ObjectRefToInstanceID(inst.Instance),
		},
		RegisteredBy: inst.RegisteredBy,
		RegisteredTs: UnixTime(t),
	}
}

func apiRefToInfo(r *repopb.Ref) RefInfo {
	var t time.Time
	if r.ModifiedTs.IsValid() {
		t = r.ModifiedTs.AsTime()
	}
	return RefInfo{
		Ref:        r.Name,
		InstanceID: common.ObjectRefToInstanceID(r.Instance),
		ModifiedBy: r.ModifiedBy,
		ModifiedTs: UnixTime(t),
	}
}

func apiTagToInfo(t *repopb.Tag) TagInfo {
	return TagInfo{
		Tag:          common.JoinInstanceTag(t),
		RegisteredBy: t.AttachedBy,
		RegisteredTs: UnixTime(t.AttachedTs.AsTime()),
	}
}

func apiMetadataToInfo(md *repopb.InstanceMetadata) MetadataInfo {
	return MetadataInfo{
		Fingerprint: md.GetFingerprint(),
		Key:         md.GetKey(),
		Value:       md.GetValue(),
		ContentType: md.GetContentType(),
		AttachedBy:  md.GetAttachedBy(),
		AttachedTs:  UnixTime(md.GetAttachedTs().AsTime()),
	}
}

func apiDescToInfo(d *repopb.DescribeInstanceResponse) *InstanceDescription {
	desc := &InstanceDescription{
		InstanceInfo: apiInstanceToInfo(d.Instance),
	}
	if len(d.Refs) != 0 {
		desc.Refs = make([]RefInfo, len(d.Refs))
		for i, r := range d.Refs {
			desc.Refs[i] = apiRefToInfo(r)
		}
	}
	if len(d.Tags) != 0 {
		desc.Tags = make([]TagInfo, len(d.Tags))
		for i, t := range d.Tags {
			desc.Tags[i] = apiTagToInfo(t)
		}
	}
	if len(d.Metadata) != 0 {
		desc.Metadata = make([]MetadataInfo, len(d.GetMetadata()))
		for i, md := range d.GetMetadata() {
			desc.Metadata[i] = apiMetadataToInfo(md)
		}
	}
	return desc
}

func apiClientDescToInfo(d *repopb.DescribeClientResponse) *ClientDescription {
	desc := &ClientDescription{
		InstanceInfo:       apiInstanceToInfo(d.Instance),
		Size:               d.ClientSize,
		SignedURL:          d.ClientBinary.SignedUrl,
		AlternativeDigests: make([]*caspb.ObjectRef, 0, len(d.ClientRefAliases)),
	}

	// Fallback value if the server doesn't support ClientRefAliases yet.
	desc.Digest = &caspb.ObjectRef{
		HashAlgo:  caspb.HashAlgo_SHA1,
		HexDigest: d.LegacySha1,
	}

	// Pick the best supported algo as 'Digest'.
	for _, ref := range d.ClientRefAliases {
		_, supported := caspb.HashAlgo_name[int32(ref.HashAlgo)]
		if supported && ref.HashAlgo > desc.Digest.HashAlgo {
			desc.Digest = ref
		}
	}

	// Put everything else into 'AlternativeDigests'.
	for _, ref := range d.ClientRefAliases {
		if ref.HashAlgo != desc.Digest.HashAlgo {
			desc.AlternativeDigests = append(desc.AlternativeDigests, ref)
		}
	}

	return desc
}
