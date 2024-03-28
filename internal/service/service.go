package service

import (
	"context"
	"sync"
	"time"

	"github.com/Falokut/cinema_orders_service/internal/events"
	"github.com/Falokut/cinema_orders_service/internal/models"
	"github.com/Falokut/cinema_orders_service/internal/repository"
	"github.com/Falokut/cinema_orders_service/pkg/sliceutils"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CinemaOrdersService interface {
	GetOccupiedPlaces(ctx context.Context, screeningID int64) ([]models.Place, error)
	ReservePlaces(ctx context.Context, screeningID int64, places []models.Place) (string, time.Duration, error)
	ProcessOrder(ctx context.Context, reservationID, accountID string) (string, error)
	CancelReservation(ctx context.Context, reservationID string) error
	GetScreeningsOccupiedPlacesCounts(ctx context.Context, ids []int64) (map[int64]uint32, error)
	GetOrders(ctx context.Context, accountID string,
		page, limit uint32, sort models.SortDTO) ([]models.OrderPreview, error)
	GetOrder(ctx context.Context, orderID, accountID string) (models.Order, error)
	RefundOrder(ctx context.Context, accountID, ordererID string) error
	RefundOrderItems(ctx context.Context, accountID, ordererID string, itemsIDs []string) error
}

type PaymentService interface {
	PreparePaymentURL(ctx context.Context, email string, amount uint32, orderID string) (string, error)
	RequestOrderRefund(ctx context.Context, percent uint32, orderID string) error
	RequestOrderItemsRefund(ctx context.Context, percent uint32, orderID string, itemsIDs []string) error
}

type ProfilesService interface {
	GetEmail(ctx context.Context) (string, error)
}
type CinemaService interface {
	GetScreeningTicketPrice(ctx context.Context, screeningID int64) (uint32, error)
	GetScreening(ctx context.Context, screeningID int64) (Screening, error)
	GetScreeningStartTime(ctx context.Context, screeningID int64) (time.Time, error)
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
	screeningID int64) (places []models.Place, err error) {
	type chanResp struct {
		seats []models.Place
		err   error
	}

	resCh := make(chan chanResp, 1)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		oplaces, oerr := s.repo.GetOccupiedPlaces(ctx, screeningID)
		resCh <- chanResp{
			seats: oplaces,
			err:   oerr,
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		rplaces, rerr := s.cache.GetReservedPlacesForScreening(ctx, screeningID)
		resCh <- chanResp{
			seats: rplaces,
			err:   rerr,
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
		seats, perr := s.repo.GetScreeningsOccupiedPlaces(ctx, ids)
		resCh <- chanResp{
			seats: seats,
			err:   perr,
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		seats, perr := s.cache.GetScreeningsReservedPlaces(ctx, ids)
		resCh <- chanResp{
			seats: seats,
			err:   perr,
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
	for screeningID, places := range places {
		placesCounts[screeningID] = uint32(len(places))
	}

	return placesCounts, nil
}

func (s *cinemaOrdersService) ReservePlaces(ctx context.Context,
	screeningID int64, toReserve []models.Place) (reserveID string, paymentTime time.Duration, err error) {
	occupied, err := s.GetOccupiedPlaces(ctx, screeningID)
	if err != nil {
		return
	}

	for i := range toReserve {
		if sliceutils.Compare(occupied, toReserve[i]) {
			err = models.Errorf(models.Conflict, "place row=%d seat=%d occupied", toReserve[i].Row, toReserve[i].Seat)
			return
		}
	}

	screening, err := s.getScreening(ctx, screeningID)
	if err != nil {
		return
	}
	if screening.StartTime.Before(time.Now()) || screening.StartTime.Equal(time.Now()) {
		err = models.Error(models.InvalidArgument, "invalid screening_id, not possible to buy tickets for the screening that has already ended")
		return
	}

	// Checking place existence in screening places
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

	id, err := s.cache.ReservePlaces(ctx, screeningID, toReserve, s.seatReservationTime)
	if err != nil {
		return
	}

	return id, s.seatReservationTime, nil
}

func (s *cinemaOrdersService) getScreening(ctx context.Context, screeningID int64) (screening Screening, err error) {
	screening, err = s.cinemaService.GetScreening(ctx, screeningID)
	if err != nil {
		return
	}

	return
}

func (s *cinemaOrdersService) GetOrder(ctx context.Context, orderID, accountID string) (order models.Order, err error) {
	return s.repo.GetOrder(ctx, orderID, accountID)
}

func (s *cinemaOrdersService) ProcessOrder(ctx context.Context,
	reservationID, accountID string) (orderID string, err error) {
	email, err := s.profilesService.GetEmail(ctx)
	if err != nil {
		return
	}

	reservedPlaces, screeningID, err := s.cache.GetReservation(ctx, reservationID)
	if err != nil {
		return
	}
	if len(reservedPlaces) == 0 {
		err = models.Error(models.NotFound, "order not found")
		return
	}

	price, err := s.cinemaService.GetScreeningTicketPrice(ctx, screeningID)
	if err != nil {
		return
	}

	orderID = uuid.NewString()
	var convertedPlaces = make([]models.ProcessOrderPlace, len(reservedPlaces))
	for i := range reservedPlaces {
		convertedPlaces[i] = models.ProcessOrderPlace{
			Place: reservedPlaces[i],
			Price: price,
		}
	}

	err = s.repo.ProcessOrder(ctx, models.ProcessOrderDTO{
		ID:          orderID,
		Places:      convertedPlaces,
		ScreeningID: screeningID,
		OwnerID:     accountID,
	})
	if err != nil {
		return
	}

	url, err := s.paymentService.PreparePaymentURL(ctx, email, price*uint32(len(reservedPlaces)), orderID)
	if err != nil {
		return
	}

	go func() {
		const deleteReservationTimeout = time.Second * 30
		ctx, cancel := context.WithTimeout(context.Background(), deleteReservationTimeout)
		defer cancel()
		err = s.cache.DeletePlacesReservation(ctx, reservationID)
		if err != nil {
			s.logger.Errorf("error while deleting reservation %v", err)
		}
	}()

	go s.sendOrderCreatedNotification(context.Background(), accountID, email, orderID)

	return url, nil
}

func (s *cinemaOrdersService) sendOrderCreatedNotification(ctx context.Context, accountID, email, orderID string) {
	order, err := s.GetOrder(ctx, orderID, accountID)
	if err != nil {
		s.logger.Error("error while getting order err=", err)
		return
	}

	err = s.orderEvents.OrderCreated(ctx, email, order)
	if err == nil {
		return
	}

	s.logger.Errorf("error while sending order in mq err=%v trying cancel order", err)
	statuses, err := s.repo.GetOrderItemsStatuses(ctx, orderID)
	if err != nil {
		s.logger.Error("error while getting order items err=", err)
		return
	}

	if len(statuses) != 1 {
		s.logger.Error("error while getting order items err=can't get items statuses")
		return
	}

	// Expecting that when paying, all the statuses of the items in 1 transaction will change
	if statuses[0] == models.OrderItemStatusPaid {
		err = s.paymentService.RequestOrderRefund(ctx, uint32(Full), orderID)
		if err != nil {
			s.logger.Error("error while requesting order items refund err=", err)
		}
	} else if statuses[0] == models.OrderItemStatusPaymentRequired {
		err = s.repo.CancelOrder(ctx, orderID)
		if err != nil {
			s.logger.Error("error while requesting order items cancelation err=", err)
		}
	}
}

func (s *cinemaOrdersService) CancelReservation(ctx context.Context, reservationID string) error {
	return s.cache.DeletePlacesReservation(ctx, reservationID)
}

func (s *cinemaOrdersService) GetOrders(ctx context.Context, accountID string, page, limit uint32,
	sort models.SortDTO) ([]models.OrderPreview, error) {
	return s.repo.GetOrders(ctx, accountID, page, limit, sort)
}

func (s *cinemaOrdersService) getOrderScreeningID(ctx context.Context, accountID, orderID string) (int64, error) {
	return s.repo.GetOrderScreeningID(ctx, accountID, orderID)
}

const (
	day = time.Hour * 24
)

type RefundPercent uint32

const (
	Full     RefundPercent = 100
	Half     RefundPercent = 50
	OneThird RefundPercent = 30
)

func (s *cinemaOrdersService) getRefundPercent(ctx context.Context, accountID, orderID string) (percent uint32, err error) {
	id, err := s.getOrderScreeningID(ctx, accountID, orderID)
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
		return uint32(Full), nil
	case timeBeforeScreening <= 5*day && timeBeforeScreening > 3*day:
		return uint32(Half), nil
	default:
		return uint32(OneThird), nil
	}
}

func (s *cinemaOrdersService) RefundOrder(ctx context.Context, accountID, orderID string) (err error) {
	percent, err := s.getRefundPercent(ctx, accountID, orderID)
	if err != nil {
		return
	}

	err = s.paymentService.RequestOrderRefund(ctx, percent, orderID)
	return
}

func (s *cinemaOrdersService) RefundOrderItems(ctx context.Context,
	accountID, orderID string, itemsIDs []string) (err error) {
	percent, err := s.getRefundPercent(ctx, accountID, orderID)
	if err != nil {
		return
	}

	return s.paymentService.RequestOrderItemsRefund(ctx, percent, orderID, itemsIDs)
}
