// Package modeldpb contains the modeld gRPC wire bindings.
//
// This file is a hand-written equivalent of the tiny generated surface in
// modeld.proto. It exists because the development environment may not have
// protoc installed; make proto can replace it once code generation is wired into
// CI. Payloads use a registered gRPC JSON codec so the transport is live without
// generated protobuf descriptors.
package modeldpb

import (
	"context"
	"encoding/json"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/status"
)

const (
	// CodecName is the gRPC content subtype used by the hand-written bindings.
	CodecName = "modeldjson"

	modelRepoServiceName = "modeld.v1.ModelRepo"
)

func init() {
	encoding.RegisterCodec(jsonCodec{})
}

type jsonCodec struct{}

func (jsonCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (jsonCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func (jsonCodec) Name() string { return CodecName }

type ListBackendsRequest struct{}

type ListBackendsResponse struct {
	BackendIds []string `json:"backend_ids,omitempty"`
}

func (r *ListBackendsResponse) GetBackendIds() []string {
	if r == nil {
		return nil
	}
	return r.BackendIds
}

type ListModelsRequest struct {
	BackendId string `json:"backend_id,omitempty"`
}

func (r *ListModelsRequest) GetBackendId() string {
	if r == nil {
		return ""
	}
	return r.BackendId
}

type BackendSpec struct {
	Type    string `json:"type,omitempty"`
	BaseUrl string `json:"base_url,omitempty"`
	ApiKey  string `json:"api_key,omitempty"`
}

type RegisterBackendRequest struct {
	BackendId string       `json:"backend_id,omitempty"`
	Spec      *BackendSpec `json:"spec,omitempty"`
}

func (r *RegisterBackendRequest) GetBackendId() string {
	if r == nil {
		return ""
	}
	return r.BackendId
}

func (r *RegisterBackendRequest) GetSpec() *BackendSpec {
	if r == nil {
		return nil
	}
	return r.Spec
}

type RegisterBackendResponse struct{}

type RemoveBackendRequest struct {
	BackendId string `json:"backend_id,omitempty"`
}

func (r *RemoveBackendRequest) GetBackendId() string {
	if r == nil {
		return ""
	}
	return r.BackendId
}

type RemoveBackendResponse struct{}

type ObservedModel struct {
	Name               string            `json:"name,omitempty"`
	ContextLength      int32             `json:"context_length,omitempty"`
	ModifiedAtUnixNano int64             `json:"modified_at_unix_nano,omitempty"`
	Size               int64             `json:"size,omitempty"`
	Digest             string            `json:"digest,omitempty"`
	MaxOutputTokens    int32             `json:"max_output_tokens,omitempty"`
	CanChat            bool              `json:"can_chat,omitempty"`
	CanEmbed           bool              `json:"can_embed,omitempty"`
	CanStream          bool              `json:"can_stream,omitempty"`
	CanPrompt          bool              `json:"can_prompt,omitempty"`
	CanThink           bool              `json:"can_think,omitempty"`
	Meta               map[string]string `json:"meta,omitempty"`
}

type ListModelsResponse struct {
	Models []*ObservedModel `json:"models,omitempty"`
}

func (r *ListModelsResponse) GetModels() []*ObservedModel {
	if r == nil {
		return nil
	}
	return r.Models
}

type ModelRepoClient interface {
	RegisterBackend(ctx context.Context, in *RegisterBackendRequest, opts ...grpc.CallOption) (*RegisterBackendResponse, error)
	RemoveBackend(ctx context.Context, in *RemoveBackendRequest, opts ...grpc.CallOption) (*RemoveBackendResponse, error)
	ListBackends(ctx context.Context, in *ListBackendsRequest, opts ...grpc.CallOption) (*ListBackendsResponse, error)
	ListModels(ctx context.Context, in *ListModelsRequest, opts ...grpc.CallOption) (*ListModelsResponse, error)
}

type modelRepoClient struct {
	cc grpc.ClientConnInterface
}

func NewModelRepoClient(cc grpc.ClientConnInterface) ModelRepoClient {
	return &modelRepoClient{cc: cc}
}

func (c *modelRepoClient) RegisterBackend(ctx context.Context, in *RegisterBackendRequest, opts ...grpc.CallOption) (*RegisterBackendResponse, error) {
	out := new(RegisterBackendResponse)
	opts = append([]grpc.CallOption{grpc.CallContentSubtype(CodecName)}, opts...)
	err := c.cc.Invoke(ctx, "/"+modelRepoServiceName+"/RegisterBackend", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *modelRepoClient) RemoveBackend(ctx context.Context, in *RemoveBackendRequest, opts ...grpc.CallOption) (*RemoveBackendResponse, error) {
	out := new(RemoveBackendResponse)
	opts = append([]grpc.CallOption{grpc.CallContentSubtype(CodecName)}, opts...)
	err := c.cc.Invoke(ctx, "/"+modelRepoServiceName+"/RemoveBackend", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *modelRepoClient) ListBackends(ctx context.Context, in *ListBackendsRequest, opts ...grpc.CallOption) (*ListBackendsResponse, error) {
	out := new(ListBackendsResponse)
	opts = append([]grpc.CallOption{grpc.CallContentSubtype(CodecName)}, opts...)
	err := c.cc.Invoke(ctx, "/"+modelRepoServiceName+"/ListBackends", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *modelRepoClient) ListModels(ctx context.Context, in *ListModelsRequest, opts ...grpc.CallOption) (*ListModelsResponse, error) {
	out := new(ListModelsResponse)
	opts = append([]grpc.CallOption{grpc.CallContentSubtype(CodecName)}, opts...)
	err := c.cc.Invoke(ctx, "/"+modelRepoServiceName+"/ListModels", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type ModelRepoServer interface {
	RegisterBackend(context.Context, *RegisterBackendRequest) (*RegisterBackendResponse, error)
	RemoveBackend(context.Context, *RemoveBackendRequest) (*RemoveBackendResponse, error)
	ListBackends(context.Context, *ListBackendsRequest) (*ListBackendsResponse, error)
	ListModels(context.Context, *ListModelsRequest) (*ListModelsResponse, error)
}

type UnimplementedModelRepoServer struct{}

func (UnimplementedModelRepoServer) RegisterBackend(context.Context, *RegisterBackendRequest) (*RegisterBackendResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RegisterBackend not implemented")
}

func (UnimplementedModelRepoServer) RemoveBackend(context.Context, *RemoveBackendRequest) (*RemoveBackendResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RemoveBackend not implemented")
}

func (UnimplementedModelRepoServer) ListBackends(context.Context, *ListBackendsRequest) (*ListBackendsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListBackends not implemented")
}

func (UnimplementedModelRepoServer) ListModels(context.Context, *ListModelsRequest) (*ListModelsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListModels not implemented")
}

func RegisterModelRepoServer(s grpc.ServiceRegistrar, srv ModelRepoServer) {
	s.RegisterService(&ModelRepo_ServiceDesc, srv)
}

func _ModelRepo_RegisterBackend_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(RegisterBackendRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ModelRepoServer).RegisterBackend(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/" + modelRepoServiceName + "/RegisterBackend",
	}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(ModelRepoServer).RegisterBackend(ctx, req.(*RegisterBackendRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ModelRepo_RemoveBackend_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(RemoveBackendRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ModelRepoServer).RemoveBackend(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/" + modelRepoServiceName + "/RemoveBackend",
	}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(ModelRepoServer).RemoveBackend(ctx, req.(*RemoveBackendRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ModelRepo_ListBackends_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(ListBackendsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ModelRepoServer).ListBackends(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/" + modelRepoServiceName + "/ListBackends",
	}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(ModelRepoServer).ListBackends(ctx, req.(*ListBackendsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ModelRepo_ListModels_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(ListModelsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ModelRepoServer).ListModels(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/" + modelRepoServiceName + "/ListModels",
	}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(ModelRepoServer).ListModels(ctx, req.(*ListModelsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var ModelRepo_ServiceDesc = grpc.ServiceDesc{
	ServiceName: modelRepoServiceName,
	HandlerType: (*ModelRepoServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "RegisterBackend",
			Handler:    _ModelRepo_RegisterBackend_Handler,
		},
		{
			MethodName: "RemoveBackend",
			Handler:    _ModelRepo_RemoveBackend_Handler,
		},
		{
			MethodName: "ListBackends",
			Handler:    _ModelRepo_ListBackends_Handler,
		},
		{
			MethodName: "ListModels",
			Handler:    _ModelRepo_ListModels_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "modeld.proto",
}
