package repository

import (
	"context"
	"time"

	"github.com/Falokut/cinema_orders_service/internal/models"
)

type DBConfig struct {
	Host     string `yaml:"host" env:"DB_HOST"`
	Port     string `yaml:"port" env:"DB_PORT"`
	Username string `yaml:"username" env:"DB_USERNAME"`
	Password string `yaml:"password" env:"DB_PASSWORD"`
	DBName   string `yaml:"db_name" env:"DB_NAME"`
	SSLMode  string `yaml:"ssl_mode" env:"DB_SSL_MODE"`
}

type CinemaOrdersRepository interface {
	GetOccupiedPlaces(ctx context.Context, screeningId int64) ([]models.Place, error)
	ProcessOrder(ctx context.Context, order models.ProcessOrderDTO) error
	GetScreeningsOccupiedPlaces(ctx context.Context, ids []int64) (map[int64][]models.Place, error)
	GetOrders(ctx context.Context, accountId string, page, limit uint32, sort models.SortDTO) ([]models.OrderPreview, error)
	GetOrder(ctx context.Context, orderId, accountId string) (models.Order, error)
	GetOrderScreeningId(ctx context.Context, accountId, orderId string) (int64, error)
	GetOrderItemsStatuses(ctx context.Context, orderId string) ([]models.OrderItemStatus, error)
	CancelOrder(ctx context.Context, orderId string) error
}

type ReserveCache interface {
	ReservePlaces(ctx context.Context, screeningId int64, seats []models.Place, ttl time.Duration) (string, error)
	GetReservation(ctx context.Context, reservationId string) (seats []models.Place, screeningId int64, err error)
	DeletePlacesReservation(ctx context.Context, reservationId string) error
	GetReservedPlacesForScreening(ctx context.Context, screeningId int64) ([]models.Place, error)
	GetScreeningsReservedPlaces(ctx context.Context, ids []int64) (map[int64][]models.Place, error)
}
