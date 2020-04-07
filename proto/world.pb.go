// Code generated by protoc-gen-go. DO NOT EDIT.
// source: world.proto

package fuel

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// swagger:model
type World struct {
	CreatedAt        *string  `protobuf:"bytes,2,opt,name=createdAt" json:"createdAt,omitempty"`
	UpdatedAt        *string  `protobuf:"bytes,3,opt,name=updatedAt" json:"updatedAt,omitempty"`
	DeletedAt        *string  `protobuf:"bytes,4,opt,name=deletedAt" json:"deletedAt,omitempty"`
	Name             *string  `protobuf:"bytes,5,opt,name=name" json:"name,omitempty"`
	Owner            *string  `protobuf:"bytes,7,opt,name=owner" json:"owner,omitempty"`
	Description      *string  `protobuf:"bytes,8,opt,name=description" json:"description,omitempty"`
	Likes            *int64   `protobuf:"varint,9,opt,name=likes" json:"likes,omitempty"`
	Downloads        *int64   `protobuf:"varint,10,opt,name=downloads" json:"downloads,omitempty"`
	Filesize         *int64   `protobuf:"varint,11,opt,name=filesize" json:"filesize,omitempty"`
	UploadDate       *string  `protobuf:"bytes,12,opt,name=upload_date,json=uploadDate" json:"upload_date,omitempty"`
	ModifyDate       *string  `protobuf:"bytes,13,opt,name=modify_date,json=modifyDate" json:"modify_date,omitempty"`
	LicenseId        *uint64  `protobuf:"varint,14,opt,name=license_id,json=licenseId" json:"license_id,omitempty"`
	LicenseName      *string  `protobuf:"bytes,15,opt,name=license_name,json=licenseName" json:"license_name,omitempty"`
	LicenseUrl       *string  `protobuf:"bytes,16,opt,name=license_url,json=licenseUrl" json:"license_url,omitempty"`
	LicenseImage     *string  `protobuf:"bytes,17,opt,name=license_image,json=licenseImage" json:"license_image,omitempty"`
	Permission       *int64   `protobuf:"varint,18,opt,name=permission" json:"permission,omitempty"`
	ThumbnailUrl     *string  `protobuf:"bytes,19,opt,name=thumbnail_url,json=thumbnailUrl" json:"thumbnail_url,omitempty"`
	IsLiked          *bool    `protobuf:"varint,20,opt,name=is_liked,json=isLiked" json:"is_liked,omitempty"`
	Version          *int64   `protobuf:"varint,21,opt,name=version" json:"version,omitempty"`
	Private          *bool    `protobuf:"varint,22,opt,name=private" json:"private,omitempty"`
	Tags             []string `protobuf:"bytes,30,rep,name=tags" json:"tags,omitempty"`
	XXX_unrecognized []byte   `json:"-"`
}

func (m *World) Reset()                    { *m = World{} }
func (m *World) String() string            { return proto.CompactTextString(m) }
func (*World) ProtoMessage()               {}
func (*World) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{0} }

func (m *World) GetCreatedAt() string {
	if m != nil && m.CreatedAt != nil {
		return *m.CreatedAt
	}
	return ""
}

func (m *World) GetUpdatedAt() string {
	if m != nil && m.UpdatedAt != nil {
		return *m.UpdatedAt
	}
	return ""
}

func (m *World) GetDeletedAt() string {
	if m != nil && m.DeletedAt != nil {
		return *m.DeletedAt
	}
	return ""
}

func (m *World) GetName() string {
	if m != nil && m.Name != nil {
		return *m.Name
	}
	return ""
}

func (m *World) GetOwner() string {
	if m != nil && m.Owner != nil {
		return *m.Owner
	}
	return ""
}

func (m *World) GetDescription() string {
	if m != nil && m.Description != nil {
		return *m.Description
	}
	return ""
}

func (m *World) GetLikes() int64 {
	if m != nil && m.Likes != nil {
		return *m.Likes
	}
	return 0
}

func (m *World) GetDownloads() int64 {
	if m != nil && m.Downloads != nil {
		return *m.Downloads
	}
	return 0
}

func (m *World) GetFilesize() int64 {
	if m != nil && m.Filesize != nil {
		return *m.Filesize
	}
	return 0
}

func (m *World) GetUploadDate() string {
	if m != nil && m.UploadDate != nil {
		return *m.UploadDate
	}
	return ""
}

func (m *World) GetModifyDate() string {
	if m != nil && m.ModifyDate != nil {
		return *m.ModifyDate
	}
	return ""
}

func (m *World) GetLicenseId() uint64 {
	if m != nil && m.LicenseId != nil {
		return *m.LicenseId
	}
	return 0
}

func (m *World) GetLicenseName() string {
	if m != nil && m.LicenseName != nil {
		return *m.LicenseName
	}
	return ""
}

func (m *World) GetLicenseUrl() string {
	if m != nil && m.LicenseUrl != nil {
		return *m.LicenseUrl
	}
	return ""
}

func (m *World) GetLicenseImage() string {
	if m != nil && m.LicenseImage != nil {
		return *m.LicenseImage
	}
	return ""
}

func (m *World) GetPermission() int64 {
	if m != nil && m.Permission != nil {
		return *m.Permission
	}
	return 0
}

func (m *World) GetThumbnailUrl() string {
	if m != nil && m.ThumbnailUrl != nil {
		return *m.ThumbnailUrl
	}
	return ""
}

func (m *World) GetIsLiked() bool {
	if m != nil && m.IsLiked != nil {
		return *m.IsLiked
	}
	return false
}

func (m *World) GetVersion() int64 {
	if m != nil && m.Version != nil {
		return *m.Version
	}
	return 0
}

func (m *World) GetPrivate() bool {
	if m != nil && m.Private != nil {
		return *m.Private
	}
	return false
}

func (m *World) GetTags() []string {
	if m != nil {
		return m.Tags
	}
	return nil
}

// swagger:model
type Worlds struct {
	Worlds           []*World `protobuf:"bytes,1,rep,name=worlds" json:"worlds,omitempty"`
	XXX_unrecognized []byte   `json:"-"`
}

func (m *Worlds) Reset()                    { *m = Worlds{} }
func (m *Worlds) String() string            { return proto.CompactTextString(m) }
func (*Worlds) ProtoMessage()               {}
func (*Worlds) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{1} }

func (m *Worlds) GetWorlds() []*World {
	if m != nil {
		return m.Worlds
	}
	return nil
}

func init() {
	proto.RegisterType((*World)(nil), "fuel.World")
	proto.RegisterType((*Worlds)(nil), "fuel.Worlds")
}

func init() { proto.RegisterFile("world.proto", fileDescriptor1) }

var fileDescriptor1 = []byte{
	// 397 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x4c, 0x92, 0xcf, 0x8e, 0xd3, 0x30,
	0x10, 0x87, 0x15, 0x92, 0x6e, 0x93, 0xc9, 0x2e, 0x7f, 0xcc, 0x82, 0x0c, 0x82, 0xdd, 0xb0, 0x7b,
	0xc9, 0x85, 0x1e, 0x78, 0x03, 0x24, 0x2e, 0x95, 0x10, 0x87, 0x48, 0x88, 0x63, 0x14, 0xea, 0x69,
	0xb1, 0x70, 0xe2, 0xc8, 0x4e, 0x5a, 0xc1, 0x4b, 0xf3, 0x0a, 0x68, 0xc6, 0x49, 0xda, 0x5b, 0x7e,
	0xdf, 0x37, 0x99, 0x91, 0xc7, 0x86, 0xfc, 0x64, 0x9d, 0x51, 0x9b, 0xde, 0xd9, 0xc1, 0x8a, 0x64,
	0x3f, 0xa2, 0x79, 0xf8, 0x97, 0xc0, 0xea, 0x07, 0x51, 0xf1, 0x0e, 0xb2, 0x9d, 0xc3, 0x66, 0x40,
	0xf5, 0x79, 0x90, 0x4f, 0x8a, 0xa8, 0xcc, 0xaa, 0x33, 0x20, 0x3b, 0xf6, 0x6a, 0xb2, 0x71, 0xb0,
	0x0b, 0x20, 0xab, 0xd0, 0x60, 0xb0, 0x49, 0xb0, 0x0b, 0x10, 0x02, 0x92, 0xae, 0x69, 0x51, 0xae,
	0x58, 0xf0, 0xb7, 0xb8, 0x85, 0x95, 0x3d, 0x75, 0xe8, 0xe4, 0x9a, 0x61, 0x08, 0xa2, 0x80, 0x5c,
	0xa1, 0xdf, 0x39, 0xdd, 0x0f, 0xda, 0x76, 0x32, 0x65, 0x77, 0x89, 0xe8, 0x3f, 0xa3, 0x7f, 0xa3,
	0x97, 0x59, 0x11, 0x95, 0x71, 0x15, 0x02, 0xcf, 0xb7, 0xa7, 0xce, 0xd8, 0x46, 0x79, 0x09, 0x6c,
	0xce, 0x40, 0xbc, 0x85, 0x74, 0xaf, 0x0d, 0x7a, 0xfd, 0x17, 0x65, 0xce, 0x72, 0xc9, 0xe2, 0x1e,
	0xf2, 0xb1, 0xa7, 0xb2, 0x9a, 0xce, 0x22, 0xaf, 0x79, 0x22, 0x04, 0xf4, 0xa5, 0x19, 0xb8, 0xa0,
	0xb5, 0x4a, 0xef, 0xff, 0x84, 0x82, 0x9b, 0x50, 0x10, 0x10, 0x17, 0xbc, 0x07, 0x30, 0x7a, 0x87,
	0x9d, 0xc7, 0x5a, 0x2b, 0xf9, 0xb4, 0x88, 0xca, 0xa4, 0xca, 0x26, 0xb2, 0x55, 0xe2, 0x03, 0x5c,
	0xcf, 0x9a, 0x97, 0xf0, 0x2c, 0x9c, 0x69, 0x62, 0xdf, 0x68, 0x17, 0xf7, 0x30, 0xc7, 0x7a, 0x74,
	0x46, 0x3e, 0x0f, 0x23, 0x26, 0xf4, 0xdd, 0x19, 0xf1, 0x08, 0x37, 0xcb, 0x88, 0xb6, 0x39, 0xa0,
	0x7c, 0xc1, 0x25, 0x73, 0xe3, 0x2d, 0x31, 0x71, 0x07, 0xd0, 0xa3, 0x6b, 0xb5, 0xf7, 0xb4, 0x3a,
	0xc1, 0xe7, 0xbc, 0x20, 0xd4, 0x64, 0xf8, 0x35, 0xb6, 0x3f, 0xbb, 0x46, 0x1b, 0x9e, 0xf3, 0x32,
	0x34, 0x59, 0x20, 0x4d, 0x7a, 0x03, 0xa9, 0xf6, 0x35, 0x2d, 0x55, 0xc9, 0xdb, 0x22, 0x2a, 0xd3,
	0x6a, 0xad, 0xfd, 0x57, 0x8a, 0x42, 0xc2, 0xfa, 0x88, 0x8e, 0x9b, 0xbf, 0xe2, 0xe6, 0x73, 0x24,
	0xd3, 0x3b, 0x7d, 0xa4, 0xf5, 0xbc, 0x0e, 0xff, 0x4c, 0x91, 0x6e, 0x7e, 0x68, 0x0e, 0x5e, 0xde,
	0x15, 0x31, 0xdd, 0x3c, 0x7d, 0x3f, 0x7c, 0x84, 0x2b, 0x7e, 0x70, 0x5e, 0x3c, 0xc2, 0x15, 0x3f,
	0x48, 0x2f, 0xa3, 0x22, 0x2e, 0xf3, 0x4f, 0xf9, 0x86, 0x9e, 0xe4, 0x86, 0x6d, 0x35, 0xa9, 0xff,
	0x01, 0x00, 0x00, 0xff, 0xff, 0x5b, 0xf7, 0x50, 0xa2, 0xb4, 0x02, 0x00, 0x00,
}