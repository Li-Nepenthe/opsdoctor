// 本文件：把 protobuf 请求接到库存业务并映射状态码。
//
// 本文件位于网络契约和业务对象之间，不直接使用 database/sql。
// 先看 GRPCServer 的嵌入方式，再沿 GetStock/Deduct/Release 追到 Service；最后读 grpcError，理解为什么只向客户端返回固定错误消息。

package inventory

import (
	"context"
	"errors"

	inventoryv1 "github.com/Li-Nepenthe/opsdoctor/gen/inventory/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPCServer 只适配协议；嵌入生成的 Unimplemented 实现，让未来新增 RPC 时仍能向前兼容。
// Go 的嵌入会提升被嵌入类型的方法，作用类似组合，不是 Java 的类继承。
type GRPCServer struct {
	inventoryv1.UnimplementedInventoryServiceServer          // 必须按值嵌入，避免未初始化的嵌入指针造成 panic。
	service                                         *Service // service 执行业务规则，本层不直接访问数据库。
}

// NewGRPCServer 把业务对象接到 RPC 适配层；app 再把这个适配器注册到 gRPC 服务器。
func NewGRPCServer(service *Service) *GRPCServer {
	return &GRPCServer{service: service}
}

// GetStock 转换领域快照；生成的 Get 方法允许 nil 请求，缺失字段按零值进入业务校验。
func (server *GRPCServer) GetStock(ctx context.Context, request *inventoryv1.GetStockRequest) (*inventoryv1.GetStockResponse, error) {
	stock, err := server.service.GetStock(ctx, request.GetSkuId())
	if err != nil {
		return nil, grpcError(err)
	}
	return &inventoryv1.GetStockResponse{SkuId: stock.SKUID, Available: stock.Available}, nil
}

// Deduct 沿用 gRPC 收到的 ctx；不能换成 Background，否则调用方 deadline 会在这里丢失。
func (server *GRPCServer) Deduct(ctx context.Context, request *inventoryv1.DeductRequest) (*inventoryv1.DeductResponse, error) {
	if err := server.service.Deduct(ctx, request.GetSkuId(), request.GetQuantity()); err != nil {
		return nil, grpcError(err)
	}
	return &inventoryv1.DeductResponse{}, nil
}

// Release 成功返回空消息；不要把无字段误认为无结果，gRPC OK 本身就是操作成功的反馈。
func (server *GRPCServer) Release(ctx context.Context, request *inventoryv1.ReleaseRequest) (*inventoryv1.ReleaseResponse, error) {
	if err := server.service.Release(ctx, request.GetSkuId(), request.GetQuantity()); err != nil {
		return nil, grpcError(err)
	}
	return &inventoryv1.ReleaseResponse{}, nil
}

// grpcError 集中落实第 5.8.1 节契约；错误消息只使用固定中文，不能把驱动详情泄露给客户端。
func grpcError(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		// 超时优先于其他分类，避免把取消连接造成的网络错误误报为数据库不可用。
		return status.Error(codes.DeadlineExceeded, "库存请求超时")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "库存请求已取消")
	case errors.Is(err, ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, "库存参数不合法或归还超出范围")
	case errors.Is(err, ErrNotFound):
		return status.Error(codes.NotFound, "商品不存在")
	case errors.Is(err, ErrInsufficientStock):
		return status.Error(codes.FailedPrecondition, "库存不足")
	case errors.Is(err, ErrUnavailable):
		return status.Error(codes.Unavailable, "库存存储暂时不可用")
	default:
		// 未知错误不能伪装成可重试；固定消息也避免把 SQL、连接地址或凭据返回给用户。
		return status.Error(codes.Internal, "库存服务内部错误")
	}
}
