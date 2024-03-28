package cinemaservice

import (
	"context"
	"errors"
	"time"

	"github.com/Falokut/cinema_orders_service/internal/config"
	"github.com/Falokut/cinema_orders_service/internal/models"
	"github.com/Falokut/cinema_orders_service/internal/service"
	cinema_service "github.com/Falokut/cinema_service/pkg/cinema_service/v1/protos"
	"github.com/grpc-ecosystem/grpc-opentracing/go/otgrpc"
	"github.com/opentracing/opentracing-go"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type CinemaService struct {
	conn   *grpc.ClientConn
	client cinema_service.CinemaServiceV1Client
	logger *logrus.Logger
}

func NewCinemaService(addr string,
	cfg config.ConnectionSecureConfig, logger *logrus.Logger) (*CinemaService, error) {
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

	return &CinemaService{
		client: cinema_service.NewCinemaServiceV1Client(conn),
		conn:   conn,
		logger: logger,
	}, nil
}

func (s *CinemaService) Shutdown() {
	if s.conn == nil {
		return
	}

	if err := s.conn.Close(); err != nil {
		s.logger.Error("cinema service error while closing connection", err.Error())
	}
}

func (s *CinemaService) GetScreeningTicketPrice(ctx context.Context, screeningID int64) (price uint32, err error) {
	defer s.handleError(ctx, &err, "GetScreeningTicketPrice")

	res, err := s.client.GetScreening(ctx, &cinema_service.GetScreeningRequest{
		ScreeningID: screeningID,
		Mask:        &fieldmaskpb.FieldMask{Paths: []string{"ticket_price"}},
	})

	if err != nil {
		return
	}

	return uint32(res.TicketPrice.Value), nil
}

func (s *CinemaService) GetScreening(ctx context.Context, screeningID int64) (screening service.Screening, err error) {
	defer s.handleError(ctx, &err, "GetScreening")

	mask := &fieldmaskpb.FieldMask{}
	mask.Paths = []string{"hall_configuration", "start_time"}
	res, err := s.client.GetScreening(ctx, &cinema_service.GetScreeningRequest{
		ScreeningID: screeningID,
		Mask:        mask})

	if err != nil {
		return
	}

	var places = make([]models.Place, len(res.HallConfiguration.Place))
	for i := range res.HallConfiguration.Place {
		places[i] = models.Place{
			Row:  res.HallConfiguration.Place[i].Row,
			Seat: res.HallConfiguration.Place[i].Seat,
		}
	}

	startTime, _ := time.Parse(time.RFC3339, res.StartTime.FormattedTimestamp)
	return service.Screening{
		Places:    places,
		StartTime: startTime,
	}, nil
}

func (s *CinemaService) GetScreeningStartTime(ctx context.Context, screeningID int64) (startTime time.Time, err error) {
	defer s.handleError(ctx, &err, "GetScreeningStartTime")

	res, err := s.client.GetScreening(ctx, &cinema_service.GetScreeningRequest{
		ScreeningID: screeningID,
		Mask:        &fieldmaskpb.FieldMask{Paths: []string{"start_time"}},
	})
	if err != nil {
		return
	}

	startTime, _ = time.Parse(time.RFC3339, res.StartTime.FormattedTimestamp)
	return startTime, nil
}

func (s *CinemaService) logError(err error, functionName string) {
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
		).Error("cinema service error occurred")
	} else {
		s.logger.WithFields(
			logrus.Fields{
				"error.function.name": functionName,
				"error.msg":           err.Error,
			},
		).Error("cinema service error occurred")
	}
}

func (s *CinemaService) handleError(ctx context.Context, err *error, functionName string) {
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

	e := *err
	s.logError(*err, functionName)
	switch status.Code(*err) {
	case codes.Canceled:
		*err = models.Error(models.Canceled, e.Error())
	case codes.DeadlineExceeded:
		*err = models.Error(models.DeadlineExceeded, e.Error())
	case codes.Internal:
		*err = models.Error(models.Internal, "")
	case codes.NotFound:
		*err = models.Error(models.NotFound, "screening with specified id not found")
	default:
		*err = models.Error(models.Unknown, e.Error())
	}
}
