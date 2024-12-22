// Package proto defines the gRPC service and message definitions
// for the Task Manager application.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.35.2
// 	protoc        v3.12.4
// source: proto/taskmanager.proto

package proto

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

// TaskPriority represents the priority level of a task.
type TaskPriority int32

const (
	// LOW indicates a low priority task.
	TaskPriority_LOW TaskPriority = 0
	// MEDIUM indicates a medium priority task.
	TaskPriority_MEDIUM TaskPriority = 1
	// HIGH indicates a high priority task.
	TaskPriority_HIGH TaskPriority = 2
)

// Enum value maps for TaskPriority.
var (
	TaskPriority_name = map[int32]string{
		0: "LOW",
		1: "MEDIUM",
		2: "HIGH",
	}
	TaskPriority_value = map[string]int32{
		"LOW":    0,
		"MEDIUM": 1,
		"HIGH":   2,
	}
)

func (x TaskPriority) Enum() *TaskPriority {
	p := new(TaskPriority)
	*p = x
	return p
}

func (x TaskPriority) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (TaskPriority) Descriptor() protoreflect.EnumDescriptor {
	return file_proto_taskmanager_proto_enumTypes[0].Descriptor()
}

func (TaskPriority) Type() protoreflect.EnumType {
	return &file_proto_taskmanager_proto_enumTypes[0]
}

func (x TaskPriority) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use TaskPriority.Descriptor instead.
func (TaskPriority) EnumDescriptor() ([]byte, []int) {
	return file_proto_taskmanager_proto_rawDescGZIP(), []int{0}
}

// TaskRequest represents a request to submit a new task.
type TaskRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// task_description is a brief description of the task.
	TaskDescription string `protobuf:"bytes,1,opt,name=task_description,json=taskDescription,proto3" json:"task_description,omitempty"`
	// priority indicates the priority level of the task.
	Priority TaskPriority `protobuf:"varint,2,opt,name=priority,proto3,enum=proto.TaskPriority" json:"priority,omitempty"`
	// number1 is the first number associated with the task.
	Number1 int32 `protobuf:"varint,3,opt,name=number1,proto3" json:"number1,omitempty"`
	// number2 is the second number associated with the task.
	Number2 int32 `protobuf:"varint,4,opt,name=number2,proto3" json:"number2,omitempty"`
}

func (x *TaskRequest) Reset() {
	*x = TaskRequest{}
	mi := &file_proto_taskmanager_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TaskRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TaskRequest) ProtoMessage() {}

func (x *TaskRequest) ProtoReflect() protoreflect.Message {
	mi := &file_proto_taskmanager_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TaskRequest.ProtoReflect.Descriptor instead.
func (*TaskRequest) Descriptor() ([]byte, []int) {
	return file_proto_taskmanager_proto_rawDescGZIP(), []int{0}
}

func (x *TaskRequest) GetTaskDescription() string {
	if x != nil {
		return x.TaskDescription
	}
	return ""
}

func (x *TaskRequest) GetPriority() TaskPriority {
	if x != nil {
		return x.Priority
	}
	return TaskPriority_LOW
}

func (x *TaskRequest) GetNumber1() int32 {
	if x != nil {
		return x.Number1
	}
	return 0
}

func (x *TaskRequest) GetNumber2() int32 {
	if x != nil {
		return x.Number2
	}
	return 0
}

// TaskResponse represents a response after submitting a task.
type TaskResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// task_id is the unique identifier of the submitted task.
	TaskId string `protobuf:"bytes,1,opt,name=task_id,json=taskId,proto3" json:"task_id,omitempty"`
}

func (x *TaskResponse) Reset() {
	*x = TaskResponse{}
	mi := &file_proto_taskmanager_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TaskResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TaskResponse) ProtoMessage() {}

func (x *TaskResponse) ProtoReflect() protoreflect.Message {
	mi := &file_proto_taskmanager_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TaskResponse.ProtoReflect.Descriptor instead.
func (*TaskResponse) Descriptor() ([]byte, []int) {
	return file_proto_taskmanager_proto_rawDescGZIP(), []int{1}
}

func (x *TaskResponse) GetTaskId() string {
	if x != nil {
		return x.TaskId
	}
	return ""
}

// StatusRequest represents a request to get the status of a task.
type StatusRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// task_id is the unique identifier of the task whose status is being requested.
	TaskId string `protobuf:"bytes,1,opt,name=task_id,json=taskId,proto3" json:"task_id,omitempty"`
}

func (x *StatusRequest) Reset() {
	*x = StatusRequest{}
	mi := &file_proto_taskmanager_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *StatusRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StatusRequest) ProtoMessage() {}

func (x *StatusRequest) ProtoReflect() protoreflect.Message {
	mi := &file_proto_taskmanager_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StatusRequest.ProtoReflect.Descriptor instead.
func (*StatusRequest) Descriptor() ([]byte, []int) {
	return file_proto_taskmanager_proto_rawDescGZIP(), []int{2}
}

func (x *StatusRequest) GetTaskId() string {
	if x != nil {
		return x.TaskId
	}
	return ""
}

// StatusResponse represents the status of a task.
type StatusResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// status is the current status of the task.
	Status string `protobuf:"bytes,1,opt,name=status,proto3" json:"status,omitempty"`
}

func (x *StatusResponse) Reset() {
	*x = StatusResponse{}
	mi := &file_proto_taskmanager_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *StatusResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StatusResponse) ProtoMessage() {}

func (x *StatusResponse) ProtoReflect() protoreflect.Message {
	mi := &file_proto_taskmanager_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StatusResponse.ProtoReflect.Descriptor instead.
func (*StatusResponse) Descriptor() ([]byte, []int) {
	return file_proto_taskmanager_proto_rawDescGZIP(), []int{3}
}

func (x *StatusResponse) GetStatus() string {
	if x != nil {
		return x.Status
	}
	return ""
}

var File_proto_taskmanager_proto protoreflect.FileDescriptor

var file_proto_taskmanager_proto_rawDesc = []byte{
	0x0a, 0x17, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x74, 0x61, 0x73, 0x6b, 0x6d, 0x61, 0x6e, 0x61,
	0x67, 0x65, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x05, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x22, 0x9d, 0x01, 0x0a, 0x0b, 0x54, 0x61, 0x73, 0x6b, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x12, 0x29, 0x0a, 0x10, 0x74, 0x61, 0x73, 0x6b, 0x5f, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70,
	0x74, 0x69, 0x6f, 0x6e, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0f, 0x74, 0x61, 0x73, 0x6b,
	0x44, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x2f, 0x0a, 0x08, 0x70,
	0x72, 0x69, 0x6f, 0x72, 0x69, 0x74, 0x79, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x13, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x54, 0x61, 0x73, 0x6b, 0x50, 0x72, 0x69, 0x6f, 0x72, 0x69,
	0x74, 0x79, 0x52, 0x08, 0x70, 0x72, 0x69, 0x6f, 0x72, 0x69, 0x74, 0x79, 0x12, 0x18, 0x0a, 0x07,
	0x6e, 0x75, 0x6d, 0x62, 0x65, 0x72, 0x31, 0x18, 0x03, 0x20, 0x01, 0x28, 0x05, 0x52, 0x07, 0x6e,
	0x75, 0x6d, 0x62, 0x65, 0x72, 0x31, 0x12, 0x18, 0x0a, 0x07, 0x6e, 0x75, 0x6d, 0x62, 0x65, 0x72,
	0x32, 0x18, 0x04, 0x20, 0x01, 0x28, 0x05, 0x52, 0x07, 0x6e, 0x75, 0x6d, 0x62, 0x65, 0x72, 0x32,
	0x22, 0x27, 0x0a, 0x0c, 0x54, 0x61, 0x73, 0x6b, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x12, 0x17, 0x0a, 0x07, 0x74, 0x61, 0x73, 0x6b, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x06, 0x74, 0x61, 0x73, 0x6b, 0x49, 0x64, 0x22, 0x28, 0x0a, 0x0d, 0x53, 0x74, 0x61,
	0x74, 0x75, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x17, 0x0a, 0x07, 0x74, 0x61,
	0x73, 0x6b, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x74, 0x61, 0x73,
	0x6b, 0x49, 0x64, 0x22, 0x28, 0x0a, 0x0e, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x16, 0x0a, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2a, 0x2d, 0x0a,
	0x0c, 0x54, 0x61, 0x73, 0x6b, 0x50, 0x72, 0x69, 0x6f, 0x72, 0x69, 0x74, 0x79, 0x12, 0x07, 0x0a,
	0x03, 0x4c, 0x4f, 0x57, 0x10, 0x00, 0x12, 0x0a, 0x0a, 0x06, 0x4d, 0x45, 0x44, 0x49, 0x55, 0x4d,
	0x10, 0x01, 0x12, 0x08, 0x0a, 0x04, 0x48, 0x49, 0x47, 0x48, 0x10, 0x02, 0x32, 0x87, 0x01, 0x0a,
	0x0b, 0x54, 0x61, 0x73, 0x6b, 0x4d, 0x61, 0x6e, 0x61, 0x67, 0x65, 0x72, 0x12, 0x35, 0x0a, 0x0a,
	0x53, 0x75, 0x62, 0x6d, 0x69, 0x74, 0x54, 0x61, 0x73, 0x6b, 0x12, 0x12, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x2e, 0x54, 0x61, 0x73, 0x6b, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x13,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x54, 0x61, 0x73, 0x6b, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x12, 0x41, 0x0a, 0x10, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x54, 0x61, 0x73,
	0x6b, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x14, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e,
	0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x15, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x30, 0x01, 0x42, 0x28, 0x5a, 0x26, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62,
	0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x6d, 0x61, 0x63, 0x69, 0x65, 0x6b, 0x62, 0x32, 0x2f, 0x74, 0x61,
	0x73, 0x6b, 0x2d, 0x6d, 0x61, 0x6e, 0x61, 0x67, 0x65, 0x72, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_proto_taskmanager_proto_rawDescOnce sync.Once
	file_proto_taskmanager_proto_rawDescData = file_proto_taskmanager_proto_rawDesc
)

func file_proto_taskmanager_proto_rawDescGZIP() []byte {
	file_proto_taskmanager_proto_rawDescOnce.Do(func() {
		file_proto_taskmanager_proto_rawDescData = protoimpl.X.CompressGZIP(file_proto_taskmanager_proto_rawDescData)
	})
	return file_proto_taskmanager_proto_rawDescData
}

var file_proto_taskmanager_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_proto_taskmanager_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_proto_taskmanager_proto_goTypes = []any{
	(TaskPriority)(0),      // 0: proto.TaskPriority
	(*TaskRequest)(nil),    // 1: proto.TaskRequest
	(*TaskResponse)(nil),   // 2: proto.TaskResponse
	(*StatusRequest)(nil),  // 3: proto.StatusRequest
	(*StatusResponse)(nil), // 4: proto.StatusResponse
}
var file_proto_taskmanager_proto_depIdxs = []int32{
	0, // 0: proto.TaskRequest.priority:type_name -> proto.TaskPriority
	1, // 1: proto.TaskManager.SubmitTask:input_type -> proto.TaskRequest
	3, // 2: proto.TaskManager.StreamTaskStatus:input_type -> proto.StatusRequest
	2, // 3: proto.TaskManager.SubmitTask:output_type -> proto.TaskResponse
	4, // 4: proto.TaskManager.StreamTaskStatus:output_type -> proto.StatusResponse
	3, // [3:5] is the sub-list for method output_type
	1, // [1:3] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_proto_taskmanager_proto_init() }
func file_proto_taskmanager_proto_init() {
	if File_proto_taskmanager_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_proto_taskmanager_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_proto_taskmanager_proto_goTypes,
		DependencyIndexes: file_proto_taskmanager_proto_depIdxs,
		EnumInfos:         file_proto_taskmanager_proto_enumTypes,
		MessageInfos:      file_proto_taskmanager_proto_msgTypes,
	}.Build()
	File_proto_taskmanager_proto = out.File
	file_proto_taskmanager_proto_rawDesc = nil
	file_proto_taskmanager_proto_goTypes = nil
	file_proto_taskmanager_proto_depIdxs = nil
}
