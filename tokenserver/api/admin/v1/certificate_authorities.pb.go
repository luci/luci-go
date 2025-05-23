// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.5
// 	protoc        v6.30.2
// source: go.chromium.org/luci/tokenserver/api/admin/v1/certificate_authorities.proto

package admin

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// FetchCRLRequest identifies a name of CA to fetch CRL for.
type FetchCRLRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Cn            string                 `protobuf:"bytes,1,opt,name=cn,proto3" json:"cn,omitempty"`        // Common Name of the CA
	Force         bool                   `protobuf:"varint,2,opt,name=force,proto3" json:"force,omitempty"` // fetch and parse CRL even if we have it already
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *FetchCRLRequest) Reset() {
	*x = FetchCRLRequest{}
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *FetchCRLRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FetchCRLRequest) ProtoMessage() {}

func (x *FetchCRLRequest) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FetchCRLRequest.ProtoReflect.Descriptor instead.
func (*FetchCRLRequest) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDescGZIP(), []int{0}
}

func (x *FetchCRLRequest) GetCn() string {
	if x != nil {
		return x.Cn
	}
	return ""
}

func (x *FetchCRLRequest) GetForce() bool {
	if x != nil {
		return x.Force
	}
	return false
}

// FetchCRLResponse is returned by FetchCRL.
type FetchCRLResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	CrlStatus     *CRLStatus             `protobuf:"bytes,1,opt,name=crl_status,json=crlStatus,proto3" json:"crl_status,omitempty"` // status of the CRL after the fetch
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *FetchCRLResponse) Reset() {
	*x = FetchCRLResponse{}
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *FetchCRLResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FetchCRLResponse) ProtoMessage() {}

func (x *FetchCRLResponse) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FetchCRLResponse.ProtoReflect.Descriptor instead.
func (*FetchCRLResponse) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDescGZIP(), []int{1}
}

func (x *FetchCRLResponse) GetCrlStatus() *CRLStatus {
	if x != nil {
		return x.CrlStatus
	}
	return nil
}

// ListCAsResponse is returned by ListCAs.
type ListCAsResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Cn            []string               `protobuf:"bytes,1,rep,name=cn,proto3" json:"cn,omitempty"` // Common Name of the CA
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ListCAsResponse) Reset() {
	*x = ListCAsResponse{}
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ListCAsResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ListCAsResponse) ProtoMessage() {}

func (x *ListCAsResponse) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ListCAsResponse.ProtoReflect.Descriptor instead.
func (*ListCAsResponse) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDescGZIP(), []int{2}
}

func (x *ListCAsResponse) GetCn() []string {
	if x != nil {
		return x.Cn
	}
	return nil
}

// GetCAStatusRequest identifies a name of CA to fetch.
type GetCAStatusRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Cn            string                 `protobuf:"bytes,1,opt,name=cn,proto3" json:"cn,omitempty"` // Common Name of the CA
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetCAStatusRequest) Reset() {
	*x = GetCAStatusRequest{}
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetCAStatusRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetCAStatusRequest) ProtoMessage() {}

func (x *GetCAStatusRequest) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetCAStatusRequest.ProtoReflect.Descriptor instead.
func (*GetCAStatusRequest) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDescGZIP(), []int{3}
}

func (x *GetCAStatusRequest) GetCn() string {
	if x != nil {
		return x.Cn
	}
	return ""
}

// GetCAStatusResponse is returned by GetCAStatus method.
//
// If requested CA doesn't exist, all fields are empty.
type GetCAStatusResponse struct {
	state         protoimpl.MessageState      `protogen:"open.v1"`
	Config        *CertificateAuthorityConfig `protobuf:"bytes,1,opt,name=config,proto3" json:"config,omitempty"`                           // current config
	Cert          string                      `protobuf:"bytes,2,opt,name=cert,proto3" json:"cert,omitempty"`                               // pem-encoded CA certificate
	Removed       bool                        `protobuf:"varint,3,opt,name=removed,proto3" json:"removed,omitempty"`                        // true if this CA was removed from the config
	Ready         bool                        `protobuf:"varint,4,opt,name=ready,proto3" json:"ready,omitempty"`                            // true if this CA is ready for usage
	AddedRev      string                      `protobuf:"bytes,5,opt,name=added_rev,json=addedRev,proto3" json:"added_rev,omitempty"`       // config rev when this CA appeared
	UpdatedRev    string                      `protobuf:"bytes,6,opt,name=updated_rev,json=updatedRev,proto3" json:"updated_rev,omitempty"` // config rev when this CA was updated
	RemovedRev    string                      `protobuf:"bytes,7,opt,name=removed_rev,json=removedRev,proto3" json:"removed_rev,omitempty"` // config rev when this CA was removed
	CrlStatus     *CRLStatus                  `protobuf:"bytes,8,opt,name=crl_status,json=crlStatus,proto3" json:"crl_status,omitempty"`    // last known status of the CRL for this CA
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetCAStatusResponse) Reset() {
	*x = GetCAStatusResponse{}
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetCAStatusResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetCAStatusResponse) ProtoMessage() {}

func (x *GetCAStatusResponse) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetCAStatusResponse.ProtoReflect.Descriptor instead.
func (*GetCAStatusResponse) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDescGZIP(), []int{4}
}

func (x *GetCAStatusResponse) GetConfig() *CertificateAuthorityConfig {
	if x != nil {
		return x.Config
	}
	return nil
}

func (x *GetCAStatusResponse) GetCert() string {
	if x != nil {
		return x.Cert
	}
	return ""
}

func (x *GetCAStatusResponse) GetRemoved() bool {
	if x != nil {
		return x.Removed
	}
	return false
}

func (x *GetCAStatusResponse) GetReady() bool {
	if x != nil {
		return x.Ready
	}
	return false
}

func (x *GetCAStatusResponse) GetAddedRev() string {
	if x != nil {
		return x.AddedRev
	}
	return ""
}

func (x *GetCAStatusResponse) GetUpdatedRev() string {
	if x != nil {
		return x.UpdatedRev
	}
	return ""
}

func (x *GetCAStatusResponse) GetRemovedRev() string {
	if x != nil {
		return x.RemovedRev
	}
	return ""
}

func (x *GetCAStatusResponse) GetCrlStatus() *CRLStatus {
	if x != nil {
		return x.CrlStatus
	}
	return nil
}

// IsRevokedCertRequest contains a name of the CA and a cert serial number.
type IsRevokedCertRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Ca            string                 `protobuf:"bytes,1,opt,name=ca,proto3" json:"ca,omitempty"` // Common Name of the CA
	Sn            string                 `protobuf:"bytes,2,opt,name=sn,proto3" json:"sn,omitempty"` // cert's serial number (big.Int encoded as a decimal string)
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *IsRevokedCertRequest) Reset() {
	*x = IsRevokedCertRequest{}
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[5]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *IsRevokedCertRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*IsRevokedCertRequest) ProtoMessage() {}

func (x *IsRevokedCertRequest) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[5]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use IsRevokedCertRequest.ProtoReflect.Descriptor instead.
func (*IsRevokedCertRequest) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDescGZIP(), []int{5}
}

func (x *IsRevokedCertRequest) GetCa() string {
	if x != nil {
		return x.Ca
	}
	return ""
}

func (x *IsRevokedCertRequest) GetSn() string {
	if x != nil {
		return x.Sn
	}
	return ""
}

// IsRevokedCertResponse is returned by IsRevokedCert
type IsRevokedCertResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Revoked       bool                   `protobuf:"varint,1,opt,name=revoked,proto3" json:"revoked,omitempty"` // true if the cert with given SN is in CRL
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *IsRevokedCertResponse) Reset() {
	*x = IsRevokedCertResponse{}
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[6]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *IsRevokedCertResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*IsRevokedCertResponse) ProtoMessage() {}

func (x *IsRevokedCertResponse) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[6]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use IsRevokedCertResponse.ProtoReflect.Descriptor instead.
func (*IsRevokedCertResponse) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDescGZIP(), []int{6}
}

func (x *IsRevokedCertResponse) GetRevoked() bool {
	if x != nil {
		return x.Revoked
	}
	return false
}

// CheckCertificateRequest contains a pem encoded certificate to check.
type CheckCertificateRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	CertPem       string                 `protobuf:"bytes,1,opt,name=cert_pem,json=certPem,proto3" json:"cert_pem,omitempty"` // pem encoded certificate to check for validity
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *CheckCertificateRequest) Reset() {
	*x = CheckCertificateRequest{}
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[7]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *CheckCertificateRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CheckCertificateRequest) ProtoMessage() {}

func (x *CheckCertificateRequest) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[7]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CheckCertificateRequest.ProtoReflect.Descriptor instead.
func (*CheckCertificateRequest) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDescGZIP(), []int{7}
}

func (x *CheckCertificateRequest) GetCertPem() string {
	if x != nil {
		return x.CertPem
	}
	return ""
}

// CheckCertificateResponse is returned by CheckCertificate.
type CheckCertificateResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	IsValid       bool                   `protobuf:"varint,1,opt,name=is_valid,json=isValid,proto3" json:"is_valid,omitempty"`                  // true when certificate is valid
	InvalidReason string                 `protobuf:"bytes,2,opt,name=invalid_reason,json=invalidReason,proto3" json:"invalid_reason,omitempty"` // a reason for certificate invalidity if it is invalid
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *CheckCertificateResponse) Reset() {
	*x = CheckCertificateResponse{}
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[8]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *CheckCertificateResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CheckCertificateResponse) ProtoMessage() {}

func (x *CheckCertificateResponse) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[8]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CheckCertificateResponse.ProtoReflect.Descriptor instead.
func (*CheckCertificateResponse) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDescGZIP(), []int{8}
}

func (x *CheckCertificateResponse) GetIsValid() bool {
	if x != nil {
		return x.IsValid
	}
	return false
}

func (x *CheckCertificateResponse) GetInvalidReason() string {
	if x != nil {
		return x.InvalidReason
	}
	return ""
}

// CRLStatus describes the latest known state of imported CRL.
type CRLStatus struct {
	state             protoimpl.MessageState `protogen:"open.v1"`
	LastUpdateTime    *timestamppb.Timestamp `protobuf:"bytes,1,opt,name=last_update_time,json=lastUpdateTime,proto3" json:"last_update_time,omitempty"`           // time when CRL was generated by the CA
	LastFetchTime     *timestamppb.Timestamp `protobuf:"bytes,2,opt,name=last_fetch_time,json=lastFetchTime,proto3" json:"last_fetch_time,omitempty"`              // time when CRL was fetched
	LastFetchEtag     string                 `protobuf:"bytes,3,opt,name=last_fetch_etag,json=lastFetchEtag,proto3" json:"last_fetch_etag,omitempty"`              // etag of last successfully fetched CRL
	RevokedCertsCount int64                  `protobuf:"varint,4,opt,name=revoked_certs_count,json=revokedCertsCount,proto3" json:"revoked_certs_count,omitempty"` // number of revoked certificates in the CRL
	unknownFields     protoimpl.UnknownFields
	sizeCache         protoimpl.SizeCache
}

func (x *CRLStatus) Reset() {
	*x = CRLStatus{}
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[9]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *CRLStatus) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CRLStatus) ProtoMessage() {}

func (x *CRLStatus) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes[9]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CRLStatus.ProtoReflect.Descriptor instead.
func (*CRLStatus) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDescGZIP(), []int{9}
}

func (x *CRLStatus) GetLastUpdateTime() *timestamppb.Timestamp {
	if x != nil {
		return x.LastUpdateTime
	}
	return nil
}

func (x *CRLStatus) GetLastFetchTime() *timestamppb.Timestamp {
	if x != nil {
		return x.LastFetchTime
	}
	return nil
}

func (x *CRLStatus) GetLastFetchEtag() string {
	if x != nil {
		return x.LastFetchEtag
	}
	return ""
}

func (x *CRLStatus) GetRevokedCertsCount() int64 {
	if x != nil {
		return x.RevokedCertsCount
	}
	return 0
}

var File_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDesc = string([]byte{
	0x0a, 0x4b, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x73, 0x65, 0x72, 0x76,
	0x65, 0x72, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2f, 0x76, 0x31, 0x2f,
	0x63, 0x65, 0x72, 0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74, 0x65, 0x5f, 0x61, 0x75, 0x74, 0x68,
	0x6f, 0x72, 0x69, 0x74, 0x69, 0x65, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x11, 0x74,
	0x6f, 0x6b, 0x65, 0x6e, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2e, 0x61, 0x64, 0x6d, 0x69, 0x6e,
	0x1a, 0x1b, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75,
	0x66, 0x2f, 0x65, 0x6d, 0x70, 0x74, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1f, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x74,
	0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x3a,
	0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f,
	0x6c, 0x75, 0x63, 0x69, 0x2f, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72,
	0x2f, 0x61, 0x70, 0x69, 0x2f, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2f, 0x76, 0x31, 0x2f, 0x63, 0x6f,
	0x6e, 0x66, 0x69, 0x67, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x37, 0x0a, 0x0f, 0x46, 0x65,
	0x74, 0x63, 0x68, 0x43, 0x52, 0x4c, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x0e, 0x0a,
	0x02, 0x63, 0x6e, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x63, 0x6e, 0x12, 0x14, 0x0a,
	0x05, 0x66, 0x6f, 0x72, 0x63, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x08, 0x52, 0x05, 0x66, 0x6f,
	0x72, 0x63, 0x65, 0x22, 0x4f, 0x0a, 0x10, 0x46, 0x65, 0x74, 0x63, 0x68, 0x43, 0x52, 0x4c, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x3b, 0x0a, 0x0a, 0x63, 0x72, 0x6c, 0x5f, 0x73,
	0x74, 0x61, 0x74, 0x75, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1c, 0x2e, 0x74, 0x6f,
	0x6b, 0x65, 0x6e, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2e, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2e,
	0x43, 0x52, 0x4c, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x09, 0x63, 0x72, 0x6c, 0x53, 0x74,
	0x61, 0x74, 0x75, 0x73, 0x22, 0x21, 0x0a, 0x0f, 0x4c, 0x69, 0x73, 0x74, 0x43, 0x41, 0x73, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x0e, 0x0a, 0x02, 0x63, 0x6e, 0x18, 0x01, 0x20,
	0x03, 0x28, 0x09, 0x52, 0x02, 0x63, 0x6e, 0x22, 0x24, 0x0a, 0x12, 0x47, 0x65, 0x74, 0x43, 0x41,
	0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x0e, 0x0a,
	0x02, 0x63, 0x6e, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x63, 0x6e, 0x22, 0xbc, 0x02,
	0x0a, 0x13, 0x47, 0x65, 0x74, 0x43, 0x41, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x45, 0x0a, 0x06, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x2d, 0x2e, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x73, 0x65, 0x72,
	0x76, 0x65, 0x72, 0x2e, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2e, 0x43, 0x65, 0x72, 0x74, 0x69, 0x66,
	0x69, 0x63, 0x61, 0x74, 0x65, 0x41, 0x75, 0x74, 0x68, 0x6f, 0x72, 0x69, 0x74, 0x79, 0x43, 0x6f,
	0x6e, 0x66, 0x69, 0x67, 0x52, 0x06, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x12, 0x12, 0x0a, 0x04,
	0x63, 0x65, 0x72, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x63, 0x65, 0x72, 0x74,
	0x12, 0x18, 0x0a, 0x07, 0x72, 0x65, 0x6d, 0x6f, 0x76, 0x65, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28,
	0x08, 0x52, 0x07, 0x72, 0x65, 0x6d, 0x6f, 0x76, 0x65, 0x64, 0x12, 0x14, 0x0a, 0x05, 0x72, 0x65,
	0x61, 0x64, 0x79, 0x18, 0x04, 0x20, 0x01, 0x28, 0x08, 0x52, 0x05, 0x72, 0x65, 0x61, 0x64, 0x79,
	0x12, 0x1b, 0x0a, 0x09, 0x61, 0x64, 0x64, 0x65, 0x64, 0x5f, 0x72, 0x65, 0x76, 0x18, 0x05, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x08, 0x61, 0x64, 0x64, 0x65, 0x64, 0x52, 0x65, 0x76, 0x12, 0x1f, 0x0a,
	0x0b, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65, 0x64, 0x5f, 0x72, 0x65, 0x76, 0x18, 0x06, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x0a, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65, 0x64, 0x52, 0x65, 0x76, 0x12, 0x1f,
	0x0a, 0x0b, 0x72, 0x65, 0x6d, 0x6f, 0x76, 0x65, 0x64, 0x5f, 0x72, 0x65, 0x76, 0x18, 0x07, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x0a, 0x72, 0x65, 0x6d, 0x6f, 0x76, 0x65, 0x64, 0x52, 0x65, 0x76, 0x12,
	0x3b, 0x0a, 0x0a, 0x63, 0x72, 0x6c, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x18, 0x08, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x1c, 0x2e, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x73, 0x65, 0x72, 0x76, 0x65,
	0x72, 0x2e, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2e, 0x43, 0x52, 0x4c, 0x53, 0x74, 0x61, 0x74, 0x75,
	0x73, 0x52, 0x09, 0x63, 0x72, 0x6c, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x22, 0x36, 0x0a, 0x14,
	0x49, 0x73, 0x52, 0x65, 0x76, 0x6f, 0x6b, 0x65, 0x64, 0x43, 0x65, 0x72, 0x74, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x12, 0x0e, 0x0a, 0x02, 0x63, 0x61, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x02, 0x63, 0x61, 0x12, 0x0e, 0x0a, 0x02, 0x73, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x02, 0x73, 0x6e, 0x22, 0x31, 0x0a, 0x15, 0x49, 0x73, 0x52, 0x65, 0x76, 0x6f, 0x6b, 0x65,
	0x64, 0x43, 0x65, 0x72, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x18, 0x0a,
	0x07, 0x72, 0x65, 0x76, 0x6f, 0x6b, 0x65, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x08, 0x52, 0x07,
	0x72, 0x65, 0x76, 0x6f, 0x6b, 0x65, 0x64, 0x22, 0x34, 0x0a, 0x17, 0x43, 0x68, 0x65, 0x63, 0x6b,
	0x43, 0x65, 0x72, 0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x12, 0x19, 0x0a, 0x08, 0x63, 0x65, 0x72, 0x74, 0x5f, 0x70, 0x65, 0x6d, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x63, 0x65, 0x72, 0x74, 0x50, 0x65, 0x6d, 0x22, 0x5c, 0x0a,
	0x18, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x43, 0x65, 0x72, 0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74,
	0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x19, 0x0a, 0x08, 0x69, 0x73, 0x5f,
	0x76, 0x61, 0x6c, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x08, 0x52, 0x07, 0x69, 0x73, 0x56,
	0x61, 0x6c, 0x69, 0x64, 0x12, 0x25, 0x0a, 0x0e, 0x69, 0x6e, 0x76, 0x61, 0x6c, 0x69, 0x64, 0x5f,
	0x72, 0x65, 0x61, 0x73, 0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0d, 0x69, 0x6e,
	0x76, 0x61, 0x6c, 0x69, 0x64, 0x52, 0x65, 0x61, 0x73, 0x6f, 0x6e, 0x22, 0xed, 0x01, 0x0a, 0x09,
	0x43, 0x52, 0x4c, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x44, 0x0a, 0x10, 0x6c, 0x61, 0x73,
	0x74, 0x5f, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x52,
	0x0e, 0x6c, 0x61, 0x73, 0x74, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x54, 0x69, 0x6d, 0x65, 0x12,
	0x42, 0x0a, 0x0f, 0x6c, 0x61, 0x73, 0x74, 0x5f, 0x66, 0x65, 0x74, 0x63, 0x68, 0x5f, 0x74, 0x69,
	0x6d, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c,
	0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73,
	0x74, 0x61, 0x6d, 0x70, 0x52, 0x0d, 0x6c, 0x61, 0x73, 0x74, 0x46, 0x65, 0x74, 0x63, 0x68, 0x54,
	0x69, 0x6d, 0x65, 0x12, 0x26, 0x0a, 0x0f, 0x6c, 0x61, 0x73, 0x74, 0x5f, 0x66, 0x65, 0x74, 0x63,
	0x68, 0x5f, 0x65, 0x74, 0x61, 0x67, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0d, 0x6c, 0x61,
	0x73, 0x74, 0x46, 0x65, 0x74, 0x63, 0x68, 0x45, 0x74, 0x61, 0x67, 0x12, 0x2e, 0x0a, 0x13, 0x72,
	0x65, 0x76, 0x6f, 0x6b, 0x65, 0x64, 0x5f, 0x63, 0x65, 0x72, 0x74, 0x73, 0x5f, 0x63, 0x6f, 0x75,
	0x6e, 0x74, 0x18, 0x04, 0x20, 0x01, 0x28, 0x03, 0x52, 0x11, 0x72, 0x65, 0x76, 0x6f, 0x6b, 0x65,
	0x64, 0x43, 0x65, 0x72, 0x74, 0x73, 0x43, 0x6f, 0x75, 0x6e, 0x74, 0x32, 0xe3, 0x03, 0x0a, 0x16,
	0x43, 0x65, 0x72, 0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74, 0x65, 0x41, 0x75, 0x74, 0x68, 0x6f,
	0x72, 0x69, 0x74, 0x69, 0x65, 0x73, 0x12, 0x53, 0x0a, 0x08, 0x46, 0x65, 0x74, 0x63, 0x68, 0x43,
	0x52, 0x4c, 0x12, 0x22, 0x2e, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72,
	0x2e, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2e, 0x46, 0x65, 0x74, 0x63, 0x68, 0x43, 0x52, 0x4c, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x23, 0x2e, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x73, 0x65,
	0x72, 0x76, 0x65, 0x72, 0x2e, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2e, 0x46, 0x65, 0x74, 0x63, 0x68,
	0x43, 0x52, 0x4c, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x45, 0x0a, 0x07, 0x4c,
	0x69, 0x73, 0x74, 0x43, 0x41, 0x73, 0x12, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x1a, 0x22,
	0x2e, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2e, 0x61, 0x64, 0x6d,
	0x69, 0x6e, 0x2e, 0x4c, 0x69, 0x73, 0x74, 0x43, 0x41, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x12, 0x5c, 0x0a, 0x0b, 0x47, 0x65, 0x74, 0x43, 0x41, 0x53, 0x74, 0x61, 0x74, 0x75,
	0x73, 0x12, 0x25, 0x2e, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2e,
	0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2e, 0x47, 0x65, 0x74, 0x43, 0x41, 0x53, 0x74, 0x61, 0x74, 0x75,
	0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x26, 0x2e, 0x74, 0x6f, 0x6b, 0x65, 0x6e,
	0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2e, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2e, 0x47, 0x65, 0x74,
	0x43, 0x41, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x12, 0x62, 0x0a, 0x0d, 0x49, 0x73, 0x52, 0x65, 0x76, 0x6f, 0x6b, 0x65, 0x64, 0x43, 0x65, 0x72,
	0x74, 0x12, 0x27, 0x2e, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2e,
	0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2e, 0x49, 0x73, 0x52, 0x65, 0x76, 0x6f, 0x6b, 0x65, 0x64, 0x43,
	0x65, 0x72, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x28, 0x2e, 0x74, 0x6f, 0x6b,
	0x65, 0x6e, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2e, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2e, 0x49,
	0x73, 0x52, 0x65, 0x76, 0x6f, 0x6b, 0x65, 0x64, 0x43, 0x65, 0x72, 0x74, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x12, 0x6b, 0x0a, 0x10, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x43, 0x65, 0x72,
	0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74, 0x65, 0x12, 0x2a, 0x2e, 0x74, 0x6f, 0x6b, 0x65, 0x6e,
	0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2e, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2e, 0x43, 0x68, 0x65,
	0x63, 0x6b, 0x43, 0x65, 0x72, 0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74, 0x65, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x1a, 0x2b, 0x2e, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x73, 0x65, 0x72, 0x76,
	0x65, 0x72, 0x2e, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2e, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x43, 0x65,
	0x72, 0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x42, 0x35, 0x5a, 0x33, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d,
	0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x73,
	0x65, 0x72, 0x76, 0x65, 0x72, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2f,
	0x76, 0x31, 0x3b, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDescData []byte
)

func file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDesc), len(file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDescData
}

var file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes = make([]protoimpl.MessageInfo, 10)
var file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_goTypes = []any{
	(*FetchCRLRequest)(nil),            // 0: tokenserver.admin.FetchCRLRequest
	(*FetchCRLResponse)(nil),           // 1: tokenserver.admin.FetchCRLResponse
	(*ListCAsResponse)(nil),            // 2: tokenserver.admin.ListCAsResponse
	(*GetCAStatusRequest)(nil),         // 3: tokenserver.admin.GetCAStatusRequest
	(*GetCAStatusResponse)(nil),        // 4: tokenserver.admin.GetCAStatusResponse
	(*IsRevokedCertRequest)(nil),       // 5: tokenserver.admin.IsRevokedCertRequest
	(*IsRevokedCertResponse)(nil),      // 6: tokenserver.admin.IsRevokedCertResponse
	(*CheckCertificateRequest)(nil),    // 7: tokenserver.admin.CheckCertificateRequest
	(*CheckCertificateResponse)(nil),   // 8: tokenserver.admin.CheckCertificateResponse
	(*CRLStatus)(nil),                  // 9: tokenserver.admin.CRLStatus
	(*CertificateAuthorityConfig)(nil), // 10: tokenserver.admin.CertificateAuthorityConfig
	(*timestamppb.Timestamp)(nil),      // 11: google.protobuf.Timestamp
	(*emptypb.Empty)(nil),              // 12: google.protobuf.Empty
}
var file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_depIdxs = []int32{
	9,  // 0: tokenserver.admin.FetchCRLResponse.crl_status:type_name -> tokenserver.admin.CRLStatus
	10, // 1: tokenserver.admin.GetCAStatusResponse.config:type_name -> tokenserver.admin.CertificateAuthorityConfig
	9,  // 2: tokenserver.admin.GetCAStatusResponse.crl_status:type_name -> tokenserver.admin.CRLStatus
	11, // 3: tokenserver.admin.CRLStatus.last_update_time:type_name -> google.protobuf.Timestamp
	11, // 4: tokenserver.admin.CRLStatus.last_fetch_time:type_name -> google.protobuf.Timestamp
	0,  // 5: tokenserver.admin.CertificateAuthorities.FetchCRL:input_type -> tokenserver.admin.FetchCRLRequest
	12, // 6: tokenserver.admin.CertificateAuthorities.ListCAs:input_type -> google.protobuf.Empty
	3,  // 7: tokenserver.admin.CertificateAuthorities.GetCAStatus:input_type -> tokenserver.admin.GetCAStatusRequest
	5,  // 8: tokenserver.admin.CertificateAuthorities.IsRevokedCert:input_type -> tokenserver.admin.IsRevokedCertRequest
	7,  // 9: tokenserver.admin.CertificateAuthorities.CheckCertificate:input_type -> tokenserver.admin.CheckCertificateRequest
	1,  // 10: tokenserver.admin.CertificateAuthorities.FetchCRL:output_type -> tokenserver.admin.FetchCRLResponse
	2,  // 11: tokenserver.admin.CertificateAuthorities.ListCAs:output_type -> tokenserver.admin.ListCAsResponse
	4,  // 12: tokenserver.admin.CertificateAuthorities.GetCAStatus:output_type -> tokenserver.admin.GetCAStatusResponse
	6,  // 13: tokenserver.admin.CertificateAuthorities.IsRevokedCert:output_type -> tokenserver.admin.IsRevokedCertResponse
	8,  // 14: tokenserver.admin.CertificateAuthorities.CheckCertificate:output_type -> tokenserver.admin.CheckCertificateResponse
	10, // [10:15] is the sub-list for method output_type
	5,  // [5:10] is the sub-list for method input_type
	5,  // [5:5] is the sub-list for extension type_name
	5,  // [5:5] is the sub-list for extension extendee
	0,  // [0:5] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_init() }
func file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_init() {
	if File_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto != nil {
		return
	}
	file_go_chromium_org_luci_tokenserver_api_admin_v1_config_proto_init()
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDesc), len(file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   10,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto = out.File
	file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_goTypes = nil
	file_go_chromium_org_luci_tokenserver_api_admin_v1_certificate_authorities_proto_depIdxs = nil
}
