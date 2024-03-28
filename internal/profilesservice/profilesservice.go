package grpcservice

import (
	"context"
	"errors"

	"github.com/Falokut/cinema_orders_service/internal/config"
	"github.com/Falokut/cinema_orders_service/internal/models"
	profiles_service "github.com/Falokut/profiles_service/pkg/profiles_service/v1/protos"
	"github.com/grpc-ecosystem/grpc-opentracing/go/otgrpc"
	"github.com/opentracing/opentracing-go"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ProfilesService struct {
	conn   *grpc.ClientConn
	client profiles_service.ProfilesServiceV1Client
	logger *logrus.Logger
}

func NewProfilesService(addr string, cfg config.ConnectionSecureConfig, logger *logrus.Logger) (*ProfilesService, error) {
	creds, err := cfg.GetGrpcTransportCredentials()
	if err != nil {
		return nil, err
	}
	conn, err := grpc.Dial(addr, creds,
		grpc.WithUnaryInterceptor(
			otgrpc.OpenTracingClientInterceptor(opentracing.GlobalTracer())),
		grpc.WithStreamInterceptor(
			otgrpc.OpenTracingStreamClientInterceptor(opentracing.GlobalTracer())),
	)
	if err != nil {
		return nil, err
	}

	return &ProfilesService{
		client: profiles_service.NewProfilesServiceV1Client(conn),
		conn:   conn,
		logger: logger,
	}, nil
}

func (s *ProfilesService) Shutdown() {
	if s.conn == nil {
		return
	}

	if err := s.conn.Close(); err != nil {
		s.logger.Error("profiles service error while closing connection", err.Error())
	}
}

// in metadata must be header X-Account-Id
func (s *ProfilesService) GetEmail(ctx context.Context) (email string, err error) {
	defer s.handleError(ctx, &err, "GetEmail")

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		err = models.Error(models.Unauthenticated, "")
		return
	}
	ctx = metadata.NewOutgoingContext(ctx, md)
	res, err := s.client.GetEmail(ctx, &emptypb.Empty{})
	if err != nil {
		return
	}

	return res.Email, nil
}

func (s *ProfilesService) logError(err error, functionName string) {
	if err == nil {
		return
	}

	var sericeErr = &models.ServiceError{}
	if errors.As(err, &sericeErr) {
		s.logger.WithFields(
			logrus.Fields{
				"error.function.name": functionName,
				"error.msg":           sericeErr.Msg,
				"code":                sericeErr.Code,
			},
		).Error("profiles service error occurred")
	} else {
		s.logger.WithFields(
			logrus.Fields{
				"error.function.name": functionName,
				"error.msg":           err.Error,
			},
		).Error("profiles service error occurred")
	}
}

func (s *ProfilesService) handleError(ctx context.Context, err *error, functionName string) {
	if ctx.Err() != nil {
		var code models.ErrorCode
		switch {
		case errors.Is(ctx.Err(), context.Canceled):
			code = models.Canceled
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			code = models.DeadlineExceeded
		}
		*err = models.Error(code, ctx.Err().Error())
		return
	}
	if err == nil || *err == nil {
		return
	}

	s.logError(*err, functionName)
	e := *err
	switch status.Code(*err) {
	case codes.Canceled:
		*err = models.Error(models.Canceled, e.Error())
	case codes.DeadlineExceeded:
		*err = models.Error(models.DeadlineExceeded, e.Error())
	case codes.Internal:
		*err = models.Error(models.Internal, "profiles service internal error")
	case codes.NotFound:
		*err = models.Error(models.NotFound, "profile not found")
	default:
		*err = models.Error(models.Unknown, e.Error())
	}
}
