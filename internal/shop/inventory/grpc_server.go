// 本文件：把库存 RPC 接到业务层，并把领域错误转换为对外稳定的状态码和固定消息。
// 从 Deduct 可以看到调用方的截止时间如何穿过服务端到达仓储。

package inventory

import (
	"context"
	"errors"

	inventoryv1 "github.com/Li-Nepenthe/opsdoctor/gen/inventory/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPCServer 将库存方法交给 Service，尚未接入的新 RPC 返回 Unimplemented。
type GRPCServer struct {
	inventoryv1.UnimplementedInventoryServiceServer
	service *Service
}

// NewGRPCServer 创建供库存服务注册的 RPC 实现。
func NewGRPCServer(service *Service) *GRPCServer {
	return &GRPCServer{service: service}
}

// GetStock 转换领域快照。生成的 Get 方法允许 nil 请求，缺失字段按零值进入业务校验。
func (server *GRPCServer) GetStock(ctx context.Context, request *inventoryv1.GetStockRequest) (*inventoryv1.GetStockResponse, error) {
	stock, err := server.service.GetStock(ctx, request.GetSkuId())
	if err != nil {
		return nil, grpcError(err)
	}
	return &inventoryv1.GetStockResponse{SkuId: stock.SKUID, Available: stock.Available}, nil
}

// Deduct 传递调用方的 ctx，改用 Background 会切断数据库操作的截止时间。
func (server *GRPCServer) Deduct(ctx context.Context, request *inventoryv1.DeductRequest) (*inventoryv1.DeductResponse, error) {
	if err := server.service.Deduct(ctx, request.GetSkuId(), request.GetQuantity()); err != nil {
		return nil, grpcError(err)
	}
	return &inventoryv1.DeductResponse{}, nil
}

// Release 成功时以 gRPC OK 确认库存已归还。
func (server *GRPCServer) Release(ctx context.Context, request *inventoryv1.ReleaseRequest) (*inventoryv1.ReleaseResponse, error) {
	if err := server.service.Release(ctx, request.GetSkuId(), request.GetQuantity()); err != nil {
		return nil, grpcError(err)
	}
	return &inventoryv1.ReleaseResponse{}, nil
}

// grpcError 向客户端返回固定消息，驱动错误只用于选择状态码。
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
		// 未知错误不能伪装成可重试。固定消息也避免把 SQL、连接地址或凭据返回给用户。
		return status.Error(codes.Internal, "库存服务内部错误")
	}
}
