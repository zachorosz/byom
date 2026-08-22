package rpc

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/validate"

	"github.com/zachorosz/byom/proto/library/v1/libraryv1connect"
	"github.com/zachorosz/byom/proto/management/v1/managementv1connect"
)

func NewHandler(logger *slog.Logger, lib *LibraryServer, mgmt *ManagementServer) http.Handler {
	opts := []connect.HandlerOption{
		connect.WithInterceptors(validate.NewInterceptor(), logErrors(logger)),
		connect.WithRecover(func(_ context.Context, spec connect.Spec, _ http.Header, p any) error {
			logger.Error("rpc panic", slog.String("procedure", spec.Procedure), slog.Any("panic", p))
			return connect.NewError(connect.CodeInternal, errPanic)
		}),
	}

	mux := http.NewServeMux()
	mux.Handle(libraryv1connect.NewLibraryServiceHandler(lib, opts...))
	mux.Handle(managementv1connect.NewManagementServiceHandler(mgmt, opts...))
	return mux
}

func logErrors(logger *slog.Logger) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			res, err := next(ctx, req)
			if err != nil {
				logger.Error("rpc failed",
					slog.String("procedure", req.Spec().Procedure),
					slog.String("code", connect.CodeOf(err).String()),
					slog.Any("error", err))
			}
			return res, err
		}
	}
}
