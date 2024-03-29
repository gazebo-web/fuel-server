// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.1
// 	protoc        (unknown)
// source: review.proto

package fuel

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

type Review_ReviewStatus int32

const (
	Review_Open   Review_ReviewStatus = 0
	Review_Merged Review_ReviewStatus = 1
	Review_Closed Review_ReviewStatus = 2
)

// Enum value maps for Review_ReviewStatus.
var (
	Review_ReviewStatus_name = map[int32]string{
		0: "Open",
		1: "Merged",
		2: "Closed",
	}
	Review_ReviewStatus_value = map[string]int32{
		"Open":   0,
		"Merged": 1,
		"Closed": 2,
	}
)

func (x Review_ReviewStatus) Enum() *Review_ReviewStatus {
	p := new(Review_ReviewStatus)
	*p = x
	return p
}

func (x Review_ReviewStatus) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Review_ReviewStatus) Descriptor() protoreflect.EnumDescriptor {
	return file_review_proto_enumTypes[0].Descriptor()
}

func (Review_ReviewStatus) Type() protoreflect.EnumType {
	return &file_review_proto_enumTypes[0]
}

func (x Review_ReviewStatus) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Do not use.
func (x *Review_ReviewStatus) UnmarshalJSON(b []byte) error {
	num, err := protoimpl.X.UnmarshalJSONEnum(x.Descriptor(), b)
	if err != nil {
		return err
	}
	*x = Review_ReviewStatus(num)
	return nil
}

// Deprecated: Use Review_ReviewStatus.Descriptor instead.
func (Review_ReviewStatus) EnumDescriptor() ([]byte, []int) {
	return file_review_proto_rawDescGZIP(), []int{0, 0}
}

// swagger:review
type Review struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	CreatedAt   *string              `protobuf:"bytes,1,opt,name=createdAt" json:"createdAt,omitempty"`
	UpdatedAt   *string              `protobuf:"bytes,2,opt,name=updatedAt" json:"updatedAt,omitempty"`
	Creator     *string              `protobuf:"bytes,3,opt,name=creator" json:"creator,omitempty"`
	Owner       *string              `protobuf:"bytes,4,opt,name=owner" json:"owner,omitempty"`
	Title       *string              `protobuf:"bytes,5,opt,name=title" json:"title,omitempty"`
	Description *string              `protobuf:"bytes,6,opt,name=description" json:"description,omitempty"`
	Branch      *string              `protobuf:"bytes,7,opt,name=branch" json:"branch,omitempty"`
	Status      *Review_ReviewStatus `protobuf:"varint,8,opt,name=status,enum=fuel.Review_ReviewStatus" json:"status,omitempty"`
	Reviewers   []string             `protobuf:"bytes,9,rep,name=reviewers" json:"reviewers,omitempty"`
	Approvals   []string             `protobuf:"bytes,10,rep,name=approvals" json:"approvals,omitempty"`
	Private     *bool                `protobuf:"varint,11,opt,name=private" json:"private,omitempty"`
}

func (x *Review) Reset() {
	*x = Review{}
	if protoimpl.UnsafeEnabled {
		mi := &file_review_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Review) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Review) ProtoMessage() {}

func (x *Review) ProtoReflect() protoreflect.Message {
	mi := &file_review_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Review.ProtoReflect.Descriptor instead.
func (*Review) Descriptor() ([]byte, []int) {
	return file_review_proto_rawDescGZIP(), []int{0}
}

func (x *Review) GetCreatedAt() string {
	if x != nil && x.CreatedAt != nil {
		return *x.CreatedAt
	}
	return ""
}

func (x *Review) GetUpdatedAt() string {
	if x != nil && x.UpdatedAt != nil {
		return *x.UpdatedAt
	}
	return ""
}

func (x *Review) GetCreator() string {
	if x != nil && x.Creator != nil {
		return *x.Creator
	}
	return ""
}

func (x *Review) GetOwner() string {
	if x != nil && x.Owner != nil {
		return *x.Owner
	}
	return ""
}

func (x *Review) GetTitle() string {
	if x != nil && x.Title != nil {
		return *x.Title
	}
	return ""
}

func (x *Review) GetDescription() string {
	if x != nil && x.Description != nil {
		return *x.Description
	}
	return ""
}

func (x *Review) GetBranch() string {
	if x != nil && x.Branch != nil {
		return *x.Branch
	}
	return ""
}

func (x *Review) GetStatus() Review_ReviewStatus {
	if x != nil && x.Status != nil {
		return *x.Status
	}
	return Review_Open
}

func (x *Review) GetReviewers() []string {
	if x != nil {
		return x.Reviewers
	}
	return nil
}

func (x *Review) GetApprovals() []string {
	if x != nil {
		return x.Approvals
	}
	return nil
}

func (x *Review) GetPrivate() bool {
	if x != nil && x.Private != nil {
		return *x.Private
	}
	return false
}

// swagger:review
type Reviews struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Reviews []*Review `protobuf:"bytes,1,rep,name=reviews" json:"reviews,omitempty"`
}

func (x *Reviews) Reset() {
	*x = Reviews{}
	if protoimpl.UnsafeEnabled {
		mi := &file_review_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Reviews) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Reviews) ProtoMessage() {}

func (x *Reviews) ProtoReflect() protoreflect.Message {
	mi := &file_review_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Reviews.ProtoReflect.Descriptor instead.
func (*Reviews) Descriptor() ([]byte, []int) {
	return file_review_proto_rawDescGZIP(), []int{1}
}

func (x *Reviews) GetReviews() []*Review {
	if x != nil {
		return x.Reviews
	}
	return nil
}

type ModelReview struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Review  *Review `protobuf:"bytes,1,req,name=review" json:"review,omitempty"`
	ModelId *uint64 `protobuf:"varint,2,req,name=model_id,json=modelId" json:"model_id,omitempty"`
}

func (x *ModelReview) Reset() {
	*x = ModelReview{}
	if protoimpl.UnsafeEnabled {
		mi := &file_review_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ModelReview) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ModelReview) ProtoMessage() {}

func (x *ModelReview) ProtoReflect() protoreflect.Message {
	mi := &file_review_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ModelReview.ProtoReflect.Descriptor instead.
func (*ModelReview) Descriptor() ([]byte, []int) {
	return file_review_proto_rawDescGZIP(), []int{2}
}

func (x *ModelReview) GetReview() *Review {
	if x != nil {
		return x.Review
	}
	return nil
}

func (x *ModelReview) GetModelId() uint64 {
	if x != nil && x.ModelId != nil {
		return *x.ModelId
	}
	return 0
}

type ModelReviews struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	ModelReviews []*ModelReview `protobuf:"bytes,1,rep,name=modelReviews" json:"modelReviews,omitempty"`
}

func (x *ModelReviews) Reset() {
	*x = ModelReviews{}
	if protoimpl.UnsafeEnabled {
		mi := &file_review_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ModelReviews) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ModelReviews) ProtoMessage() {}

func (x *ModelReviews) ProtoReflect() protoreflect.Message {
	mi := &file_review_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ModelReviews.ProtoReflect.Descriptor instead.
func (*ModelReviews) Descriptor() ([]byte, []int) {
	return file_review_proto_rawDescGZIP(), []int{3}
}

func (x *ModelReviews) GetModelReviews() []*ModelReview {
	if x != nil {
		return x.ModelReviews
	}
	return nil
}

var File_review_proto protoreflect.FileDescriptor

var file_review_proto_rawDesc = []byte{
	0x0a, 0x0c, 0x72, 0x65, 0x76, 0x69, 0x65, 0x77, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x04,
	0x66, 0x75, 0x65, 0x6c, 0x1a, 0x0b, 0x6d, 0x6f, 0x64, 0x65, 0x6c, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x22, 0xff, 0x02, 0x0a, 0x06, 0x52, 0x65, 0x76, 0x69, 0x65, 0x77, 0x12, 0x1c, 0x0a, 0x09,
	0x63, 0x72, 0x65, 0x61, 0x74, 0x65, 0x64, 0x41, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x09, 0x63, 0x72, 0x65, 0x61, 0x74, 0x65, 0x64, 0x41, 0x74, 0x12, 0x1c, 0x0a, 0x09, 0x75, 0x70,
	0x64, 0x61, 0x74, 0x65, 0x64, 0x41, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x75,
	0x70, 0x64, 0x61, 0x74, 0x65, 0x64, 0x41, 0x74, 0x12, 0x18, 0x0a, 0x07, 0x63, 0x72, 0x65, 0x61,
	0x74, 0x6f, 0x72, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x63, 0x72, 0x65, 0x61, 0x74,
	0x6f, 0x72, 0x12, 0x14, 0x0a, 0x05, 0x6f, 0x77, 0x6e, 0x65, 0x72, 0x18, 0x04, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x05, 0x6f, 0x77, 0x6e, 0x65, 0x72, 0x12, 0x14, 0x0a, 0x05, 0x74, 0x69, 0x74, 0x6c,
	0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x74, 0x69, 0x74, 0x6c, 0x65, 0x12, 0x20,
	0x0a, 0x0b, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x06, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x0b, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e,
	0x12, 0x16, 0x0a, 0x06, 0x62, 0x72, 0x61, 0x6e, 0x63, 0x68, 0x18, 0x07, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x06, 0x62, 0x72, 0x61, 0x6e, 0x63, 0x68, 0x12, 0x31, 0x0a, 0x06, 0x73, 0x74, 0x61, 0x74,
	0x75, 0x73, 0x18, 0x08, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x19, 0x2e, 0x66, 0x75, 0x65, 0x6c, 0x2e,
	0x52, 0x65, 0x76, 0x69, 0x65, 0x77, 0x2e, 0x52, 0x65, 0x76, 0x69, 0x65, 0x77, 0x53, 0x74, 0x61,
	0x74, 0x75, 0x73, 0x52, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x1c, 0x0a, 0x09, 0x72,
	0x65, 0x76, 0x69, 0x65, 0x77, 0x65, 0x72, 0x73, 0x18, 0x09, 0x20, 0x03, 0x28, 0x09, 0x52, 0x09,
	0x72, 0x65, 0x76, 0x69, 0x65, 0x77, 0x65, 0x72, 0x73, 0x12, 0x1c, 0x0a, 0x09, 0x61, 0x70, 0x70,
	0x72, 0x6f, 0x76, 0x61, 0x6c, 0x73, 0x18, 0x0a, 0x20, 0x03, 0x28, 0x09, 0x52, 0x09, 0x61, 0x70,
	0x70, 0x72, 0x6f, 0x76, 0x61, 0x6c, 0x73, 0x12, 0x18, 0x0a, 0x07, 0x70, 0x72, 0x69, 0x76, 0x61,
	0x74, 0x65, 0x18, 0x0b, 0x20, 0x01, 0x28, 0x08, 0x52, 0x07, 0x70, 0x72, 0x69, 0x76, 0x61, 0x74,
	0x65, 0x22, 0x30, 0x0a, 0x0c, 0x52, 0x65, 0x76, 0x69, 0x65, 0x77, 0x53, 0x74, 0x61, 0x74, 0x75,
	0x73, 0x12, 0x08, 0x0a, 0x04, 0x4f, 0x70, 0x65, 0x6e, 0x10, 0x00, 0x12, 0x0a, 0x0a, 0x06, 0x4d,
	0x65, 0x72, 0x67, 0x65, 0x64, 0x10, 0x01, 0x12, 0x0a, 0x0a, 0x06, 0x43, 0x6c, 0x6f, 0x73, 0x65,
	0x64, 0x10, 0x02, 0x22, 0x31, 0x0a, 0x07, 0x52, 0x65, 0x76, 0x69, 0x65, 0x77, 0x73, 0x12, 0x26,
	0x0a, 0x07, 0x72, 0x65, 0x76, 0x69, 0x65, 0x77, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32,
	0x0c, 0x2e, 0x66, 0x75, 0x65, 0x6c, 0x2e, 0x52, 0x65, 0x76, 0x69, 0x65, 0x77, 0x52, 0x07, 0x72,
	0x65, 0x76, 0x69, 0x65, 0x77, 0x73, 0x22, 0x4e, 0x0a, 0x0b, 0x4d, 0x6f, 0x64, 0x65, 0x6c, 0x52,
	0x65, 0x76, 0x69, 0x65, 0x77, 0x12, 0x24, 0x0a, 0x06, 0x72, 0x65, 0x76, 0x69, 0x65, 0x77, 0x18,
	0x01, 0x20, 0x02, 0x28, 0x0b, 0x32, 0x0c, 0x2e, 0x66, 0x75, 0x65, 0x6c, 0x2e, 0x52, 0x65, 0x76,
	0x69, 0x65, 0x77, 0x52, 0x06, 0x72, 0x65, 0x76, 0x69, 0x65, 0x77, 0x12, 0x19, 0x0a, 0x08, 0x6d,
	0x6f, 0x64, 0x65, 0x6c, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x02, 0x28, 0x04, 0x52, 0x07, 0x6d,
	0x6f, 0x64, 0x65, 0x6c, 0x49, 0x64, 0x22, 0x45, 0x0a, 0x0c, 0x4d, 0x6f, 0x64, 0x65, 0x6c, 0x52,
	0x65, 0x76, 0x69, 0x65, 0x77, 0x73, 0x12, 0x35, 0x0a, 0x0c, 0x6d, 0x6f, 0x64, 0x65, 0x6c, 0x52,
	0x65, 0x76, 0x69, 0x65, 0x77, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x11, 0x2e, 0x66,
	0x75, 0x65, 0x6c, 0x2e, 0x4d, 0x6f, 0x64, 0x65, 0x6c, 0x52, 0x65, 0x76, 0x69, 0x65, 0x77, 0x52,
	0x0c, 0x6d, 0x6f, 0x64, 0x65, 0x6c, 0x52, 0x65, 0x76, 0x69, 0x65, 0x77, 0x73, 0x42, 0x28, 0x5a,
	0x26, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x67, 0x61, 0x7a, 0x65,
	0x62, 0x6f, 0x2d, 0x77, 0x65, 0x62, 0x2f, 0x66, 0x75, 0x65, 0x6c, 0x2d, 0x73, 0x65, 0x72, 0x76,
	0x65, 0x72, 0x2f, 0x66, 0x75, 0x65, 0x6c,
}

var (
	file_review_proto_rawDescOnce sync.Once
	file_review_proto_rawDescData = file_review_proto_rawDesc
)

func file_review_proto_rawDescGZIP() []byte {
	file_review_proto_rawDescOnce.Do(func() {
		file_review_proto_rawDescData = protoimpl.X.CompressGZIP(file_review_proto_rawDescData)
	})
	return file_review_proto_rawDescData
}

var file_review_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_review_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_review_proto_goTypes = []interface{}{
	(Review_ReviewStatus)(0), // 0: fuel.Review.ReviewStatus
	(*Review)(nil),           // 1: fuel.Review
	(*Reviews)(nil),          // 2: fuel.Reviews
	(*ModelReview)(nil),      // 3: fuel.ModelReview
	(*ModelReviews)(nil),     // 4: fuel.ModelReviews
}
var file_review_proto_depIdxs = []int32{
	0, // 0: fuel.Review.status:type_name -> fuel.Review.ReviewStatus
	1, // 1: fuel.Reviews.reviews:type_name -> fuel.Review
	1, // 2: fuel.ModelReview.review:type_name -> fuel.Review
	3, // 3: fuel.ModelReviews.modelReviews:type_name -> fuel.ModelReview
	4, // [4:4] is the sub-list for method output_type
	4, // [4:4] is the sub-list for method input_type
	4, // [4:4] is the sub-list for extension type_name
	4, // [4:4] is the sub-list for extension extendee
	0, // [0:4] is the sub-list for field type_name
}

func init() { file_review_proto_init() }
func file_review_proto_init() {
	if File_review_proto != nil {
		return
	}
	file_model_proto_init()
	if !protoimpl.UnsafeEnabled {
		file_review_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Review); i {
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
		file_review_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Reviews); i {
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
		file_review_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ModelReview); i {
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
		file_review_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ModelReviews); i {
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
			RawDescriptor: file_review_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_review_proto_goTypes,
		DependencyIndexes: file_review_proto_depIdxs,
		EnumInfos:         file_review_proto_enumTypes,
		MessageInfos:      file_review_proto_msgTypes,
	}.Build()
	File_review_proto = out.File
	file_review_proto_rawDesc = nil
	file_review_proto_goTypes = nil
	file_review_proto_depIdxs = nil
}
