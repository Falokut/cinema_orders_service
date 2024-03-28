package handler

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Falokut/cinema_orders_service/internal/models"
	"github.com/Falokut/cinema_orders_service/internal/service"
	cinema_orders_service "github.com/Falokut/cinema_orders_service/pkg/cinema_orders_service/v1/protos"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type CinemaOrdersHandler struct {
	cinema_orders_service.UnimplementedCinemaOrdersServiceV1Server
	logger *logrus.Logger
	s      service.CinemaOrdersService
}

func NewCinemaOrdersHandler(logger *logrus.Logger, s service.CinemaOrdersService) *CinemaOrdersHandler {
	return &CinemaOrdersHandler{logger: logger, s: s}
}

func (h *CinemaOrdersHandler) GetOccupiedPlaces(ctx context.Context,
	in *cinema_orders_service.GetOccupiedPlacesRequest) (res *cinema_orders_service.Places, err error) {
	defer h.handleError(&err)

	places, err := h.s.GetOccupiedPlaces(ctx, in.ScreeningID)
	if err != nil {
		return
	}

	return &cinema_orders_service.Places{
		Places: convertModelsPlacesToGrpc(places),
	}, nil
}

func (h *CinemaOrdersHandler) ReservePlaces(ctx context.Context,
	in *cinema_orders_service.ReservePlacesRequest) (res *cinema_orders_service.ReservePlacesResponse, err error) {
	defer h.handleError(&err)

	if len(in.Places) > 5 || len(in.Places) < 1 {
		err = status.Error(codes.InvalidArgument, "places count must be bigger than 0 and less than or equal 5")
		return
	}

	id, timeToPay, err := h.s.ReservePlaces(ctx,
		in.ScreeningID, convertGrpcPlacesToModels(in.Places))
	if err != nil {
		return
	}

	timeToPayInMinutes := int32(math.Floor(timeToPay.Minutes()))
	return &cinema_orders_service.ReservePlacesResponse{
		ReserveID: id,
		TimeToPay: timeToPayInMinutes}, nil
}

const (
	AccountIDContext = "X-Account-Id"
)

func (h *CinemaOrdersHandler) ProcessOrder(ctx context.Context,
	in *cinema_orders_service.ProcessOrderRequest) (res *cinema_orders_service.ProcessOrderResponse, err error) {
	defer h.handleError(&err)

	accountID, ok := getAccountIDFromCtx(ctx)
	if !ok {
		err = status.Error(codes.Unauthenticated, "X-Account-Id header not specified")
		return
	}

	url, err := h.s.ProcessOrder(ctx, in.ReserveID, accountID)
	if err != nil {
		return
	}

	return &cinema_orders_service.ProcessOrderResponse{PaymentUrl: url}, nil
}

func (h *CinemaOrdersHandler) CancelReservation(ctx context.Context,
	in *cinema_orders_service.CancelReservationRequest) (res *emptypb.Empty, err error) {
	defer h.handleError(&err)

	err = h.s.CancelReservation(ctx, in.ReserveID)
	if err != nil {
		return
	}

	return &emptypb.Empty{}, nil
}

func (h *CinemaOrdersHandler) RefundOrder(ctx context.Context,
	in *cinema_orders_service.RefundOrderRequest) (res *emptypb.Empty, err error) {
	defer h.handleError(&err)
	accountID, ok := getAccountIDFromCtx(ctx)
	if !ok {
		err = status.Error(codes.Unauthenticated, "X-Account-Id header not specified")
		return
	}

	if len(in.ItemsIDs) == 0 {
		err = h.s.RefundOrder(ctx, accountID, in.OrderID)
	} else {
		err = h.s.RefundOrderItems(ctx, accountID, in.OrderID, in.ItemsIDs)
	}

	if err != nil {
		return
	}

	return &emptypb.Empty{}, nil
}

func (h *CinemaOrdersHandler) GetOrder(ctx context.Context,
	in *cinema_orders_service.GetOrderRequest) (
	res *cinema_orders_service.Order, err error) {
	defer h.handleError(&err)

	accountID, ok := getAccountIDFromCtx(ctx)
	if !ok {
		err = status.Error(codes.Unauthenticated, "X-Account-Id header not specified")
		return
	}

	order, err := h.s.GetOrder(ctx, in.OrderID, accountID)
	if err != nil {
		return
	}

	res = &cinema_orders_service.Order{
		OrderDate:   &cinema_orders_service.Timestamp{FormattedTimestamp: order.Date.Format(time.RFC3339)},
		Tickets:     make([]*cinema_orders_service.Ticket, 0, len(order.Tickets)),
		ScreeningID: order.ScreeningID,
	}
	for i := range order.Tickets {
		res.Tickets = append(res.Tickets, &cinema_orders_service.Ticket{
			TicketID: order.Tickets[i].ID,
			Place: &cinema_orders_service.Place{
				Row:  order.Tickets[i].Place.Row,
				Seat: order.Tickets[i].Place.Seat,
			},
			Price:  &cinema_orders_service.Price{Value: int32(order.Tickets[i].Price)},
			Status: OrderStatusFromModels(order.Tickets[i].Status),
		})
	}

	return
}

func (h *CinemaOrdersHandler) GetScreeningsOccupiedPlacesCounts(ctx context.Context,
	in *cinema_orders_service.GetScreeningsOccupiedPlacesCountsRequest) (
	res *cinema_orders_service.ScreeningsOccupiedPlacesCount, err error) {
	defer h.handleError(&err)

	in.ScreeningsIDs = strings.TrimSpace(strings.ReplaceAll(in.ScreeningsIDs, `"`, ""))
	if ok := checkIds(in.ScreeningsIDs); !ok {
		err = status.Error(codes.InvalidArgument,
			"screenings_ids mustn't be empty and screenings_ids ids must contains only digits and commas")
		return
	}

	ids := convertStringToInt64Slice(strings.Split(in.ScreeningsIDs, ","))
	places, err := h.s.GetScreeningsOccupiedPlacesCounts(ctx, ids)
	if err != nil {
		return
	}

	res = &cinema_orders_service.ScreeningsOccupiedPlacesCount{
		ScreeningsOccupiedPlacesCount: places,
	}
	return
}

func (h *CinemaOrdersHandler) GetOrders(ctx context.Context,
	in *cinema_orders_service.GetOrdersRequest) (res *cinema_orders_service.OrdersPreviews, err error) {
	defer h.handleError(&err)

	accountID, ok := getAccountIDFromCtx(ctx)
	if !ok {
		err = status.Error(codes.Unauthenticated, "X-Account-Id header not specified")
		return
	}

	if in.Page < 1 {
		err = status.Error(codes.InvalidArgument, "page must be bigger than or equal 1")
		return
	}

	if in.Limit < 10 || in.Limit > 100 {
		err = status.Error(codes.InvalidArgument,
			"limit must be bigger than or equal 10 an less than or equal 100")
		return
	}

	preview, err := h.s.GetOrders(ctx,
		accountID, in.Page, in.Limit, getOrdersPreviewSort(in.Sort))
	if err != nil {
		return
	}

	res =
		&cinema_orders_service.OrdersPreviews{
			Orders: convertModelsOrdersPreviewsToGrpc(preview),
		}

	return
}

func getOrdersPreviewSort(sort *cinema_orders_service.Sort) models.SortDTO {
	if sort == nil {
		return models.SortDTO{
			FieldName:    "order_date",
			SortOrdering: models.DESC,
		}
	}

	orderering := models.ASC
	if sort.Ordering == cinema_orders_service.Sort_DESC {
		orderering = models.DESC
	}

	return models.SortDTO{
		FieldName:    sort.FieldName,
		SortOrdering: orderering,
	}
}

// does not check whether the el in str is a number or not
func convertStringToInt64Slice(str []string) []int64 {
	res := make([]int64, len(str))
	for i := range str {
		num, _ := strconv.Atoi(str[i])
		res[i] = int64(num)
	}
	return res
}

func checkIds(val string) bool {
	return regexp.MustCompile(`^\d+(,\d+)*$`).MatchString(val)
}

func getAccountIDFromCtx(ctx context.Context) (string, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false
	}
	accountID := md.Get(AccountIDContext)
	if len(accountID) == 0 {
		return "", false
	}

	return accountID[0], true
}

func convertServiceErrCodeToGrpc(code models.ErrorCode) codes.Code {
	switch code {
	case models.Internal:
		return codes.Internal
	case models.InvalidArgument:
		return codes.InvalidArgument
	case models.Unauthenticated:
		return codes.Unauthenticated
	case models.Conflict:
		return codes.AlreadyExists
	case models.NotFound:
		return codes.NotFound
	case models.Canceled:
		return codes.Canceled
	case models.DeadlineExceeded:
		return codes.DeadlineExceeded
	case models.PermissionDenied:
		return codes.PermissionDenied
	default:
		return codes.Unknown
	}
}

func (h *CinemaOrdersHandler) handleError(err *error) {
	if err == nil || *err == nil {
		return
	}

	serviceErr := &models.ServiceError{}
	if errors.As(*err, &serviceErr) {
		*err = status.Error(convertServiceErrCodeToGrpc(serviceErr.Code), serviceErr.Msg)
	} else if _, ok := status.FromError(*err); !ok {
		e := *err
		*err = status.Error(codes.Unknown, e.Error())
	}
}

func convertModelsPlacesToGrpc(pl []models.Place) []*cinema_orders_service.Place {
	var res = make([]*cinema_orders_service.Place, len(pl))
	for i := range pl {
		res[i] = &cinema_orders_service.Place{
			Row:  pl[i].Row,
			Seat: pl[i].Seat,
		}
	}
	return res
}

func convertGrpcPlacesToModels(pl []*cinema_orders_service.Place) []models.Place {
	var res = make(map[models.Place]struct{}, len(pl))
	for i := range pl {
		if pl[i] == nil {
			continue
		}

		res[models.Place{
			Row:  pl[i].Row,
			Seat: pl[i].Seat,
		}] = struct{}{}
	}
	return maps.Keys(res)
}

func convertModelsOrdersPreviewsToGrpc(pr []models.OrderPreview) []*cinema_orders_service.OrderPreview {
	var res = make([]*cinema_orders_service.OrderPreview, len(pr))
	for i := range pr {
		res[i] = &cinema_orders_service.OrderPreview{
			OrderID:     pr[i].OrderID,
			OrderDate:   &cinema_orders_service.Timestamp{FormattedTimestamp: pr[i].OrderDate.Format(time.RFC3339)},
			TotalPrice:  &cinema_orders_service.Price{Value: int32(pr[i].TotalPrice)},
			ScreeningID: pr[i].ScreeningID,
		}
	}
	return res
}

func OrderStatusFromModels(st models.OrderItemStatus) (res cinema_orders_service.Status) {
	switch st {
	case models.OrderItemStatusPaid:
		res = cinema_orders_service.Status_PAID
	case models.OrderItemStatusPaymentRequired:
		res = cinema_orders_service.Status_PAYMENT_REQUIRED
	case models.OrderItemStatusRefundAwaiting:
		res = cinema_orders_service.Status_REFUND_AWAITING
	case models.OrderItemStatusCanceled:
		res = cinema_orders_service.Status_CANCELED
	case models.OrderItemStatusUsed:
		res = cinema_orders_service.Status_USED
	case models.OrderItemStatusRefunded:
		res = cinema_orders_service.Status_REFUNDED
	}
	return
}
