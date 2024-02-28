package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	cinema_service "github.com/Falokut/cinema_orders_service/internal/cinemaservice"
	"github.com/Falokut/cinema_orders_service/internal/events"
	payment_service "github.com/Falokut/cinema_orders_service/internal/paymentservice"
	profiles_service "github.com/Falokut/cinema_orders_service/internal/profilesservice"

	"github.com/Falokut/cinema_orders_service/internal/config"
	"github.com/Falokut/cinema_orders_service/internal/handler"
	mongo_repository "github.com/Falokut/cinema_orders_service/internal/repository/mongorepository"
	"github.com/Falokut/cinema_orders_service/internal/repository/rediscache"
	"github.com/Falokut/cinema_orders_service/internal/service"
	cinema_orders_service "github.com/Falokut/cinema_orders_service/pkg/cinema_orders_service/v1/protos"
	jaegerTracer "github.com/Falokut/cinema_orders_service/pkg/jaeger"
	"github.com/Falokut/cinema_orders_service/pkg/logging"
	"github.com/Falokut/cinema_orders_service/pkg/metrics"
	server "github.com/Falokut/grpc_rest_server"
	"github.com/Falokut/healthcheck"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/opentracing/opentracing-go"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

func main() {
	logging.NewEntry(logging.ConsoleOutput)
	logger := logging.GetLogger()
	cfg := config.GetConfig()

	logLevel, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		logger.Fatal(err)
	}
	logger.Logger.SetLevel(logLevel)

	tracer, closer, err := jaegerTracer.InitJaeger(cfg.JaegerConfig)
	if err != nil {
		logger.Errorf("Shutting down, error while creating tracer %v", err)
		return
	}
	logger.Info("Jaeger connected")
	defer closer.Close()
	opentracing.SetGlobalTracer(tracer)

	logger.Info("Metrics initializing")
	metric, err := metrics.CreateMetrics(cfg.PrometheusConfig.Name)
	if err != nil {
		logger.Errorf("Shutting down, error while creating metrics %v", err)
		return
	}

	shutdown := make(chan error, 1)
	go func() {
		logger.Info("Metrics server running")
		if err := metrics.RunMetricServer(cfg.PrometheusConfig.ServerConfig); err != nil {
			logger.Errorf("Shutting down, error while running metrics server %v", err)
			shutdown <- err
			return
		}
	}()

	cinemaOrdersDB, err := mongo_repository.NewMongoDB(cfg.DbConnectionString)
	if err != nil {
		logger.Errorf("Shutting down, connection to the database not established %v", err)
		return
	}
	defer cinemaOrdersDB.Disconnect(context.Background())
	cinemaRepo := mongo_repository.NewCinemaOrdersRepository(
		logger.Logger, cinemaOrdersDB, cfg.DbName)

	reserveRdb, err := rediscache.NewRedisCache(&redis.Options{
		Network:  cfg.ReserveCache.Network,
		Addr:     cfg.ReserveCache.Addr,
		Password: cfg.ReserveCache.Password,
		DB:       cfg.ReserveCache.DB,
	})
	if err != nil {
		logger.Errorf("Shutting down, connection to the reserve cache not established %v", err)
		return
	}
	defer reserveRdb.Shutdown(context.Background())
	reserveCache := rediscache.NewReserveCache(reserveRdb, logger.Logger)
	go func() {
		logger.Info("Healthcheck initializing")
		healthcheckManager := healthcheck.NewHealthManager(logger.Logger,
			[]healthcheck.HealthcheckResource{cinemaRepo, reserveCache}, cfg.HealthcheckPort, nil)
		if err := healthcheckManager.RunHealthcheckEndpoint(); err != nil {
			logger.Errorf("Shutting down, error while running healthcheck endpoint %s", err.Error())
			shutdown <- err
			return
		}
	}()

	cinemaService, err := cinema_service.NewCinemaService(
		cfg.CinemaServiceConfig.Addr,
		cfg.CinemaServiceConfig.SecureConfig,
		logger.Logger)
	if err != nil {
		logger.Errorf("Shutting down, connection to the cinema service not established %v", err)
		return
	}
	defer cinemaService.Shutdown()

	profilesService, err := profiles_service.NewProfilesService(cfg.ProfilesServiceConfig.Addr,
		cfg.ProfilesServiceConfig.SecureConfig, logger.Logger)
	if err != nil {
		logger.Errorf("Shutting down, connection to the profiles service not established %v", err)
		return
	}
	defer profilesService.Shutdown()

	ordersEvents := events.NewOrdersEvents(events.KafkaConfig{
		Brokers: cfg.OrdersEventsConfig.Brokers,
	}, logger.Logger)

	paymentService := payment_service.NewPaymentServiceStub(cfg.PaymentServiceConfig.Paymenturl,
		cfg.PaymentServiceConfig.PaymentSleepTime,
		cfg.PaymentServiceConfig.RefundSleepTime,
		cinemaRepo,
		logger.Logger,
	)

	service := service.NewCinemaOrdersService(logger.Logger,
		cinemaRepo,
		reserveCache,
		cfg.SeatReservationTime,
		paymentService,
		cinemaService,
		profilesService,
		ordersEvents,
	)

	handler := handler.NewCinemaOrdersHandler(logger.Logger, service)
	logger.Info("Server initializing")
	s := server.NewServer(logger.Logger, handler)
	go func() {
		if err := s.Run(getListenServerConfig(cfg), metric, nil, nil); err != nil {
			logger.Errorf("Shutting down, error while running server %s", err.Error())
			shutdown <- err
			return
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGHUP, syscall.SIGTERM)

	select {
	case <-quit:
		break
	case <-shutdown:
		break
	}

	s.Shutdown()
}

func getListenServerConfig(cfg *config.Config) server.Config {
	return server.Config{
		Mode:           cfg.Listen.Mode,
		Host:           cfg.Listen.Host,
		Port:           cfg.Listen.Port,
		AllowedHeaders: cfg.Listen.AllowedHeaders,
		ServiceDesc:    &cinema_orders_service.CinemaOrdersServiceV1_ServiceDesc,
		RegisterRestHandlerServer: func(ctx context.Context, mux *runtime.ServeMux, service any) error {
			serv, ok := service.(cinema_orders_service.CinemaOrdersServiceV1Server)
			if !ok {
				return errors.New("can't convert")
			}

			return cinema_orders_service.RegisterCinemaOrdersServiceV1HandlerServer(context.Background(),
				mux, serv)
		},
	}
}
