package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/Falokut/cinema_orders_service/internal/events"
	"github.com/Falokut/cinema_orders_service/internal/models"
	"github.com/Falokut/cinema_orders_service/internal/repository"
	cinema_orders_service "github.com/Falokut/cinema_orders_service/pkg/cinema_orders_service/v1/protos"
	"github.com/Falokut/cinema_orders_service/pkg/sliceutils"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CinemaOrdersService interface {
	GetOccupiedPlaces(ctx context.Context, screeningId int64) ([]models.Place, error)
	ReservePlaces(ctx context.Context, screeningId int64, places []models.Place) (string, time.Duration, error)
	ProcessOrder(ctx context.Context, reservationId, accountId string) (string, error)
	CancelReservation(ctx context.Context, reservationId string) error
	GetScreeningsOccupiedPlacesCounts(ctx context.Context, ids []int64) (map[int64]uint32, error)
	GetOrders(ctx context.Context, accountId string,
		page, limit uint32, sort models.SortDTO) ([]models.OrderPreview, error)
	GetOrder(ctx context.Context, orderId, accountId string) (models.Order, error)
	RefundOrder(ctx context.Context, accountId, ordererId string) error
	RefundOrderItems(ctx context.Context, accountId, ordererId string, itemsIds []string) error
}

type PaymentService interface {
	PreparePaymentUrl(ctx context.Context, email string, amount uint32, orderId string) (string, error)
	RequestOrderRefund(ctx context.Context, percent uint32, orderId string) error
	RequestOrderItemsRefund(ctx context.Context, percent uint32, orderId string, itemsIds []string) error
}

type ProfilesService interface {
	GetEmail(ctx context.Context) (string, error)
}
type CinemaService interface {
	GetScreeningTicketPrice(ctx context.Context, screeningId int64) (uint32, error)
	GetScreening(ctx context.Context, screeningId int64) (Screening, error)
	GetScreeningStartTime(ctx context.Context, screeningId int64) (time.Time, error)
}

type cinemaOrdersService struct {
	seatReservationTime time.Duration
	paymentService      PaymentService
	cinemaService       CinemaService
	profilesService     ProfilesService
	logger              *logrus.Logger
	repo                repository.CinemaOrdersRepository
	cache               repository.ReserveCache
	orderEvents         events.OrdersEventsMQ
}

type Screening struct {
	Places    []models.Place
	StartTime time.Time
}

func NewCinemaOrdersService(logger *logrus.Logger,
	repo repository.CinemaOrdersRepository,
	cache repository.ReserveCache,
	seatReservationTime time.Duration,
	paymentService PaymentService,
	cinemaService CinemaService,
	profilesService ProfilesService,
	orderEvents events.OrdersEventsMQ,
) *cinemaOrdersService {
	return &cinemaOrdersService{
		logger:              logger,
		cache:               cache,
		repo:                repo,
		seatReservationTime: seatReservationTime,
		paymentService:      paymentService,
		cinemaService:       cinemaService,
		profilesService:     profilesService,
		orderEvents:         orderEvents,
	}
}

func (s *cinemaOrdersService) GetOccupiedPlaces(ctx context.Context,
	screeningId int64) (places []models.Place, err error) {
	type chanResp struct {
		seats []models.Place
		err   error
	}

	resCh := make(chan chanResp, 1)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		places, err := s.repo.GetOccupiedPlaces(ctx, screeningId)
		resCh <- chanResp{
			seats: places,
			err:   err,
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		places, err := s.cache.GetReservedPlacesForScreening(ctx, screeningId)
		resCh <- chanResp{
			seats: places,
			err:   err,
		}
	}()

	go func() {
		wg.Wait()
		close(resCh)
	}()

LOOP:
	for {
		select {
		case <-ctx.Done():
			return nil, status.Error(codes.Canceled, "")
		case pl, ok := <-resCh:
			if !ok {
				break LOOP
			}
			if pl.err != nil {
				if models.Code(err) == models.NotFound {
					err = nil
					continue
				}
				return
			}
			places = sliceutils.UniqueMergeSlices(places, pl.seats...)
		}
	}

	return
}

func (s *cinemaOrdersService) GetScreeningsOccupiedPlacesCounts(ctx context.Context,
	ids []int64) (placesCounts map[int64]uint32, err error) {
	type chanResp struct {
		seats map[int64][]models.Place
		err   error
	}

	resCh := make(chan chanResp, 1)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		seats, err := s.repo.GetScreeningsOccupiedPlaces(ctx, ids)
		resCh <- chanResp{
			seats: seats,
			err:   err,
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		seats, err := s.cache.GetScreeningsReservedPlaces(ctx, ids)
		resCh <- chanResp{
			seats: seats,
			err:   err,
		}
	}()

	go func() {
		wg.Wait()
		close(resCh)
	}()

	places := make(map[int64][]models.Place, len(ids))
LOOP:
	for {
		select {
		case <-ctx.Done():
			return nil, status.Error(codes.Canceled, "")
		case pl, ok := <-resCh:
			if !ok {
				break LOOP
			}
			if pl.err != nil {
				if models.Code(err) == models.NotFound {
					err = nil
					continue
				}
				return
			}
			for key := range pl.seats {
				slice, ok := places[key]
				if !ok {
					places[key] = pl.seats[key]
					continue
				}

				places[key] = sliceutils.UniqueMergeSlices(slice, pl.seats[key]...)
			}
		}
	}
	placesCounts = make(map[int64]uint32, len(places))
	for screeningId, places := range places {
		placesCounts[screeningId] = uint32(len(places))
	}

	return placesCounts, nil
}

func (s *cinemaOrdersService) ReservePlaces(ctx context.Context,
	screeningId int64, toReserve []models.Place) (reserveId string, paymentTime time.Duration, err error) {
	occupied, err := s.GetOccupiedPlaces(ctx, screeningId)
	if err != nil {
		return
	}

	for i := range toReserve {
		if sliceutils.Compare(occupied, toReserve[i]) {
			err = models.Errorf(models.Conflict, "place row=%d seat=%d occupied", toReserve[i].Row, toReserve[i].Seat)
			return
		}
	}

	screening, err := s.getScreening(ctx, screeningId)
	if err != nil {
		return
	}
	//if screening.StartTime.Before(time.Now()) || screening.StartTime.Equal(time.Now()) {
	//err = Error(InvalidArgument, "invalid screening_id, not possible to buy tickets for the screening that has already ended")
	// 	return
	// }

	// Checking place existance in screening places
	for i := range toReserve {
		isSeatExist := false
		for j := range screening.Places {
			if toReserve[i].Row == screening.Places[j].Row &&
				toReserve[i].Seat == screening.Places[j].Seat {
				isSeatExist = true
				break
			}
		}
		if !isSeatExist {
			err = models.Errorf(models.InvalidArgument, "seat row=%d seat=%d not exist", toReserve[i].Row, toReserve[i].Seat)
			return
		}
	}

	id, err := s.cache.ReservePlaces(ctx, screeningId, toReserve, s.seatReservationTime)
	if err != nil {
		return
	}

	return id, s.seatReservationTime, nil
}

func (s *cinemaOrdersService) getScreening(ctx context.Context, screeningId int64) (screening Screening, err error) {
	screening, err = s.cinemaService.GetScreening(ctx, screeningId)
	if err != nil {
		return
	}

	return
}

func (s *cinemaOrdersService) GetOrder(ctx context.Context, orderId, accountId string) (order models.Order, err error) {
	return s.repo.GetOrder(ctx, orderId, accountId)
}

func (s *cinemaOrdersService) ProcessOrder(ctx context.Context,
	reservationId, accountId string) (orderId string, err error) {

	email, err := s.profilesService.GetEmail(ctx)
	if err != nil {
		return
	}

	reservedPlaces, screeningId, err := s.cache.GetReservation(ctx, reservationId)
	if err != nil {
		return
	}
	if len(reservedPlaces) == 0 {
		err = models.Error(models.NotFound, "order not found")
		return
	}

	price, err := s.cinemaService.GetScreeningTicketPrice(ctx, screeningId)
	if err != nil {
		return
	}

	orderId = uuid.NewString()
	var convertedPlaces = make([]models.ProcessOrderPlace, len(reservedPlaces))
	for i := range reservedPlaces {
		convertedPlaces[i] = models.ProcessOrderPlace{
			Place: reservedPlaces[i],
			Price: price,
		}
	}

	err = s.repo.ProcessOrder(ctx, models.ProcessOrderDTO{
		Id:          orderId,
		Places:      convertedPlaces,
		ScreeningId: screeningId,
		OwnerId:     accountId,
	})
	if err != nil {
		return
	}

	url, err := s.paymentService.PreparePaymentUrl(ctx, email, price*uint32(len(reservedPlaces)), orderId)
	if err != nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()
		err = s.cache.DeletePlacesReservation(ctx, reservationId)
		if err != nil {
			s.logger.Errorf("error while deleting reservation %v", err)
		}
	}()

	go s.sendOrderCreatedNotification(context.Background(), accountId, email, orderId)

	return url, nil
}

func (s *cinemaOrdersService) sendOrderCreatedNotification(ctx context.Context, accountId, email, orderId string) {
	order, err := s.GetOrder(ctx, orderId, accountId)
	if err != nil {
		s.logger.Error("error while getting order err=", err)
		return
	}

	err = s.orderEvents.OrderCreated(ctx, email, order)
	if err == nil {
		return
	}

	s.logger.Errorf("error while sending order in mq err=%v trying cancel order", err)
	statuses, err := s.repo.GetOrderItemsStatuses(ctx, orderId)
	if err != nil {
		s.logger.Error("error while getting order items err=", err)
		return
	}

	if len(statuses) != 1 {
		s.logger.Error("error while getting order items err=can't get items statuses")
		return
	}

	//It is expected that when paying, all the statuses of the items in 1 transaction will change
	if statuses[0] == models.ORDER_ITEM_STATUS_PAID {
		err = s.paymentService.RequestOrderRefund(ctx, 100, orderId)
		if err != nil {
			s.logger.Error("error while requesting order items refund err=", err)
		}
	} else if statuses[0] == models.ORDER_ITEM_STATUS_PAYMENT_REQUIRED {
		err = s.repo.CancelOrder(ctx, orderId)
		if err != nil {
			s.logger.Error("error while requesting order items cancelation err=", err)
		}
	}
}

func (s *cinemaOrdersService) CancelReservation(ctx context.Context, reservationId string) error {
	return s.cache.DeletePlacesReservation(ctx, reservationId)
}

func (s *cinemaOrdersService) GetOrders(ctx context.Context, accountId string, page, limit uint32,
	sort models.SortDTO) ([]models.OrderPreview, error) {
	return s.repo.GetOrders(ctx, accountId, page, limit, sort)
}

func (s *cinemaOrdersService) getOrderScreeningId(ctx context.Context, accountId, orderId string) (int64, error) {
	return s.repo.GetOrderScreeningId(ctx, accountId, orderId)
}

const (
	day = time.Hour * 24
)

func (s *cinemaOrdersService) getRefundPercent(ctx context.Context, accountId, orderId string) (percent uint32, err error) {
	id, err := s.getOrderScreeningId(ctx, accountId, orderId)
	if err != nil {
		return
	}

	startTime, err := s.cinemaService.GetScreeningStartTime(ctx, id)
	if err != nil {
		return
	}

	timeBeforeScreening := time.Since(startTime).Round(day)
	if startTime.After(time.Now()) || time.Now().Equal(startTime) || timeBeforeScreening < day*3 {
		err = models.Error(models.InvalidArgument, "less than 3 days remaining before screening, the order is non-refundable")
		return
	}

	switch {
	case timeBeforeScreening >= 10*day:
		return 100, nil
	case timeBeforeScreening <= 5*day && timeBeforeScreening > 3*day:
		return 50, nil
	default:
		return 30, nil
	}
}

func (s *cinemaOrdersService) RefundOrder(ctx context.Context, accountId, orderId string) (err error) {
	percent, err := s.getRefundPercent(ctx, accountId, orderId)
	if err != nil {
		return
	}

	err = s.paymentService.RequestOrderRefund(ctx, percent, orderId)
	return
}

func (s *cinemaOrdersService) RefundOrderItems(ctx context.Context,
	accountId, orderId string, itemsIds []string) (err error) {
	percent, err := s.getRefundPercent(ctx, accountId, orderId)
	if err != nil {
		return
	}

	return s.paymentService.RequestOrderItemsRefund(ctx, percent, orderId, itemsIds)
}

func OrderStatusFromString(str string) (cinema_orders_service.Status, error) {
	switch {
	default:
		return cinema_orders_service.Status(-1), errors.New("unknown status")
	case strings.EqualFold(str, cinema_orders_service.Status_PAYMENT_REQUIRED.String()):
		return cinema_orders_service.Status_PAYMENT_REQUIRED, nil
	case strings.EqualFold(str, cinema_orders_service.Status_PAID.String()):
		return cinema_orders_service.Status_PAID, nil
	case strings.EqualFold(str, cinema_orders_service.Status_CANCELLED.String()):
		return cinema_orders_service.Status_CANCELLED, nil
	case strings.EqualFold(str, cinema_orders_service.Status_REFUNDED.String()):
		return cinema_orders_service.Status_REFUNDED, nil
	case strings.EqualFold(str, cinema_orders_service.Status_REFUND_AWAITING.String()):
		return cinema_orders_service.Status_REFUND_AWAITING, nil
	case strings.EqualFold(str, cinema_orders_service.Status_USED.String()):
		return cinema_orders_service.Status_USED, nil
	}
}
