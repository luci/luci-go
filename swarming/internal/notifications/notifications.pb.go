// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.31.0
// 	protoc        v3.21.7
// source: go.chromium.org/luci/swarming/internal/notifications/notifications.proto

package notifications

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// Wire proto defining the payload of a Cloud PubSub notification, which is sent
// by RBE when the `scheduler_notification_config` is populated for an instance
// and the corresponding RBE event occurs.
type SchedulerNotification struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Exactly one of these IDs will be populated, depending on whether the
	// notification is for a completed action or reservation.
	ActionId      string `protobuf:"bytes,1,opt,name=action_id,json=actionId,proto3" json:"action_id,omitempty"`
	ReservationId string `protobuf:"bytes,2,opt,name=reservation_id,json=reservationId,proto3" json:"reservation_id,omitempty"`
	// Status code for the received event.
	StatusCode int32 `protobuf:"varint,3,opt,name=status_code,json=statusCode,proto3" json:"status_code,omitempty"`
}

func (x *SchedulerNotification) Reset() {
	*x = SchedulerNotification{}
	if protoimpl.UnsafeEnabled {
		mi := &file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SchedulerNotification) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SchedulerNotification) ProtoMessage() {}

func (x *SchedulerNotification) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SchedulerNotification.ProtoReflect.Descriptor instead.
func (*SchedulerNotification) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_rawDescGZIP(), []int{0}
}

func (x *SchedulerNotification) GetActionId() string {
	if x != nil {
		return x.ActionId
	}
	return ""
}

func (x *SchedulerNotification) GetReservationId() string {
	if x != nil {
		return x.ReservationId
	}
	return ""
}

func (x *SchedulerNotification) GetStatusCode() int32 {
	if x != nil {
		return x.StatusCode
	}
	return 0
}

var File_go_chromium_org_luci_swarming_internal_notifications_notifications_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_rawDesc = []byte{
	0x0a, 0x48, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x73, 0x77, 0x61, 0x72, 0x6d, 0x69, 0x6e, 0x67, 0x2f,
	0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x6e, 0x6f, 0x74, 0x69, 0x66, 0x69, 0x63,
	0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2f, 0x6e, 0x6f, 0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x22, 0x64, 0x65, 0x76, 0x74,
	0x6f, 0x6f, 0x6c, 0x73, 0x2e, 0x66, 0x6f, 0x75, 0x6e, 0x64, 0x72, 0x79, 0x2e, 0x61, 0x70, 0x69,
	0x2e, 0x6e, 0x6f, 0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x22, 0x7c,
	0x0a, 0x15, 0x53, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x4e, 0x6f, 0x74, 0x69, 0x66,
	0x69, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x1b, 0x0a, 0x09, 0x61, 0x63, 0x74, 0x69, 0x6f,
	0x6e, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x61, 0x63, 0x74, 0x69,
	0x6f, 0x6e, 0x49, 0x64, 0x12, 0x25, 0x0a, 0x0e, 0x72, 0x65, 0x73, 0x65, 0x72, 0x76, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0d, 0x72, 0x65,
	0x73, 0x65, 0x72, 0x76, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x49, 0x64, 0x12, 0x1f, 0x0a, 0x0b, 0x73,
	0x74, 0x61, 0x74, 0x75, 0x73, 0x5f, 0x63, 0x6f, 0x64, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x05,
	0x52, 0x0a, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x43, 0x6f, 0x64, 0x65, 0x42, 0x36, 0x5a, 0x34,
	0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f,
	0x6c, 0x75, 0x63, 0x69, 0x2f, 0x73, 0x77, 0x61, 0x72, 0x6d, 0x69, 0x6e, 0x67, 0x2f, 0x69, 0x6e,
	0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x6e, 0x6f, 0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x73, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_rawDescData = file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_rawDesc
)

func file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_rawDescData = protoimpl.X.CompressGZIP(file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_rawDescData)
	})
	return file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_rawDescData
}

var file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_goTypes = []interface{}{
	(*SchedulerNotification)(nil), // 0: devtools.foundry.api.notifications.SchedulerNotification
}
var file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_init() }
func file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_init() {
	if File_go_chromium_org_luci_swarming_internal_notifications_notifications_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SchedulerNotification); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_swarming_internal_notifications_notifications_proto = out.File
	file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_rawDesc = nil
	file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_goTypes = nil
	file_go_chromium_org_luci_swarming_internal_notifications_notifications_proto_depIdxs = nil
}