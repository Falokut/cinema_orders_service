package mongo_repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Falokut/cinema_orders_service/internal/models"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type CinemaOrdersRepository struct {
	db           *mongo.Client
	logger       *logrus.Logger
	databaseName string
}

func NewCinemaOrdersRepository(logger *logrus.Logger, db *mongo.Client,
	databaseName string) *CinemaOrdersRepository {
	return &CinemaOrdersRepository{
		logger:       logger,
		db:           db,
		databaseName: databaseName,
	}
}

const (
	ordersCollectionName = "orders"
)

func (r *CinemaOrdersRepository) PingContext(ctx context.Context) error {
	return r.db.Ping(ctx, nil)
}

const (
	ticketType = "ticket"
)

func (r *CinemaOrdersRepository) GetOccupiedPlaces(ctx context.Context, screeningId int64) (places []models.Place, err error) {
	defer r.handleError(ctx, &err, "GetOccupiedPlaces")

	collection := r.db.Database(r.databaseName).Collection(ordersCollectionName)
	filter := bson.D{
		{Key: "screening_id", Value: screeningId},
		{Key: "type", Value: ticketType},
		{Key: "status",
			Value: bson.D{
				{Key: "$nin",
					Value: bson.A{
						models.ORDER_ITEM_STATUS_CANCELLED,
						models.ORDER_ITEM_STATUS_REFUNDED,
					},
				},
			},
		},
	}

	projection := bson.D{
		{Key: "seat", Value: "$place.seat"},
		{Key: "row", Value: "$place.row"},
		{Key: "_id", Value: 0},
	}

	cur, err := collection.Find(ctx, filter, options.Find().SetProjection(projection))
	if err != nil {
		return
	}

	err = cur.All(ctx, &places)
	if err != nil {
		return
	}

	return
}

func (r *CinemaOrdersRepository) ChangeOrderStatus(ctx context.Context, orderId string, newStatus models.OrderItemStatus) (err error) {
	defer r.handleError(ctx, &err, "ChangeOrderStatus")

	allowedPreviousStatuses := models.GetAllowedPreviousOrderStatuses(newStatus)
	collection := r.db.Database(r.databaseName).Collection(ordersCollectionName)
	_, err = collection.UpdateMany(ctx, bson.D{
		{Key: "order_id", Value: orderId},
		{Key: "status",
			Value: bson.D{
				{Key: "$in", Value: allowedPreviousStatuses},
			}},
	},
		bson.D{{Key: "$set",
			Value: bson.D{
				{Key: "status", Value: string(newStatus)},
			},
		},
		})

	if err != nil {
		return
	}
	return nil
}

func (r *CinemaOrdersRepository) GetOrderItemsStatuses(ctx context.Context,
	orderId string) (statuses []models.OrderItemStatus, err error) {
	defer r.handleError(ctx, &err, "GetOrderItemsStatuses")

	collection := r.db.Database(r.databaseName).Collection(ordersCollectionName)
	pipe := bson.A{
		bson.D{{Key: "$match", Value: bson.D{{Key: "order_id", Value: orderId}}}},
		bson.D{
			{
				Key: "$group",
				Value: bson.D{
					{Key: "_id", Value: "$order_id"},
					{Key: "statuses", Value: bson.D{{Key: "$addToSet", Value: "$status"}}},
				},
			},
		},
	}

	type statusesModel struct {
		Statuses []string `bson:"statuses"`
	}

	cur, err := collection.Aggregate(ctx, pipe)
	if err != nil {
		return
	}

	var st statusesModel
	err = cur.All(ctx, &st)
	if err != nil {
		return
	}

	for i := range st.Statuses {
		status, err := models.OrderItemStatusFromString(st.Statuses[i])
		// ignoring invalid statuses
		if err != nil {
			continue
		}
		statuses = append(statuses, status)
	}
	return
}

func (r *CinemaOrdersRepository) CancelOrder(ctx context.Context, orderId string) (err error) {
	defer r.handleError(ctx, &err, "CancelOrder")

	allowedPreviousStatuses := models.GetAllowedPreviousOrderStatuses(models.ORDER_ITEM_STATUS_CANCELLED)

	session, err := r.db.StartSession()
	if err != nil {
		return
	}

	if err = session.StartTransaction(); err != nil {
		return
	}

	err = mongo.WithSession(ctx, session, func(sc mongo.SessionContext) (err error) {
		collection := r.db.Database(r.databaseName).Collection(ordersCollectionName)
		filter := bson.D{
			{Key: "order_id", Value: orderId},
			{Key: "status", Value: bson.E{Key: "$in", Value: allowedPreviousStatuses}},
		}
		_, err = collection.UpdateMany(ctx, filter,
			bson.D{{Key: "$set",
				Value: bson.D{
					{Key: "status", Value: string(models.ORDER_ITEM_STATUS_CANCELLED)},
				},
			},
			})
		if err != nil {
			return
		}
		if err = sc.CommitTransaction(ctx); err != nil {
			return
		}
		return nil
	})

	return

}
func (r *CinemaOrdersRepository) ChangeOrderItemsStatus(ctx context.Context, orderId string,
	itemsIds []string, newStatus models.OrderItemStatus) (err error) {
	defer r.handleError(ctx, &err, "ChangeOrderItemsStatus")
	allowedPreviousStatuses := models.GetAllowedPreviousOrderStatuses(newStatus)

	session, err := r.db.StartSession()
	if err != nil {
		return
	}

	if err = session.StartTransaction(); err != nil {
		return
	}

	err = mongo.WithSession(ctx, session, func(sc mongo.SessionContext) (err error) {
		collection := r.db.Database(r.databaseName).Collection(ordersCollectionName)
		filter := bson.D{
			{Key: "order_id", Value: orderId},
			{Key: "_id", Value: bson.E{Key: "$in", Value: itemsIds}},
			{Key: "status", Value: bson.E{Key: "$in", Value: allowedPreviousStatuses}},
		}
		res, err := collection.UpdateMany(ctx, filter,
			bson.D{{Key: "$set",
				Value: bson.D{
					{Key: "status", Value: string(newStatus)},
				},
			},
			})
		if err != nil {
			return
		}
		if res.ModifiedCount != int64(len(itemsIds)) {
			return models.Error(models.NotFound, "error while changing order items status")
		}
		if err = sc.CommitTransaction(ctx); err != nil {
			return
		}
		return nil
	})

	return
}

func (r *CinemaOrdersRepository) GetScreeningsOccupiedPlaces(ctx context.Context,
	ids []int64) (res map[int64][]models.Place, err error) {
	defer r.handleError(ctx, &err, "GetScreeningsOccupiedPlaces")

	collection := r.db.Database(r.databaseName).Collection(ordersCollectionName)

	pipe := bson.A{
		bson.D{
			{
				Key: "$match",
				Value: bson.D{
					{Key: "screening_id",
						Value: bson.D{{Key: "$in", Value: ids}},
					},
					{Key: "type", Value: ticketType},
					{Key: "status",
						Value: bson.D{
							{Key: "$nin",
								Value: bson.A{
									models.ORDER_ITEM_STATUS_CANCELLED,
									models.ORDER_ITEM_STATUS_REFUNDED,
								},
							},
						},
					},
				},
			},
		},
		bson.D{
			{
				Key: "$group",
				Value: bson.D{
					{Key: "_id", Value: "$screening_id"},
					{Key: "places", Value: bson.D{{Key: "$push", Value: "$place"}}},
				},
			},
		},
	}

	cur, err := collection.Aggregate(ctx, pipe)
	if err != nil {
		return
	}

	type screeningOccupiedPlaces struct {
		ScreeningId int64          `bson:"_id"`
		Places      []models.Place `bson:"places"`
	}
	var places []screeningOccupiedPlaces
	err = cur.All(ctx, &places)
	if err != nil {
		return
	}

	res = make(map[int64][]models.Place, len(places))
	for i := range places {
		res[places[i].ScreeningId] = places[i].Places
	}

	return res, nil
}

func (r *CinemaOrdersRepository) ProcessOrder(ctx context.Context, order models.ProcessOrderDTO) (err error) {
	defer r.handleError(ctx, &err, "ProcessOrder")

	tickets := make([]interface{}, len(order.Places))
	date := time.Now()
	for i := range order.Places {
		tickets[i] = struct {
			TempId  string `bson:"_id"`
			OrderId string `bson:"order_id"`
			OwnerId string `bson:"owner_id"`
			// Always be ticket
			Type        string       `bson:"type"`
			Status      string       `bson:"status"`
			Date        time.Time    `bson:"order_date"`
			ScreeningId int64        `bson:"screening_id"`
			Place       models.Place `bson:"place"`
			Price       uint32       `bson:"price"`
		}{
			TempId:      uuid.NewString(),
			OrderId:     order.Id,
			OwnerId:     order.OwnerId,
			Place:       order.Places[i].Place,
			Price:       order.Places[i].Price,
			ScreeningId: order.ScreeningId,
			Date:        date,
			Type:        ticketType,
			Status:      string(models.ORDER_ITEM_STATUS_PAYMENT_REQUIRED),
		}
	}

	session, err := r.db.StartSession()
	if err != nil {
		return
	}

	if err = session.StartTransaction(); err != nil {
		return
	}

	err = mongo.WithSession(ctx, session, func(sc mongo.SessionContext) error {
		collection := r.db.Database(r.databaseName).Collection(ordersCollectionName)
		_, err := collection.InsertMany(ctx, tickets)
		if err != nil {
			return err
		}

		if err = sc.CommitTransaction(ctx); err != nil {
			return err
		}
		return nil
	})

	return err
}

func (r *CinemaOrdersRepository) GetOrders(ctx context.Context, accountId string,
	page, limit uint32, sort models.SortDTO) (orders []models.OrderPreview, err error) {
	defer r.handleError(ctx, &err, "GetOrders")

	collection := r.db.Database(r.databaseName).Collection(ordersCollectionName)
	fieldname, ordering, err := convertSortDTOToOrderPreviewSortParams(sort)
	if err != nil {
		return
	}

	pipe := bson.A{
		bson.D{
			{Key: "$match",
				Value: bson.D{
					{Key: "owner_id", Value: accountId},
				},
			},
		},
		bson.D{
			{Key: "$group",
				Value: bson.D{
					{Key: "_id", Value: "$order_id"},
					{Key: "order_date", Value: bson.D{{Key: "$first", Value: "$order_date"}}},
					{Key: "screening_id", Value: bson.D{{Key: "$first", Value: "$screening_id"}}},
					{Key: "total_price", Value: bson.D{{Key: "$sum", Value: "$price"}}},
				},
			},
		},
		bson.D{{Key: "$sort", Value: bson.D{{Key: fieldname, Value: ordering}}}},
		bson.D{{Key: "$skip", Value: (page - 1) * limit}},
		bson.D{{Key: "$limit", Value: limit}},
	}

	cur, err := collection.Aggregate(ctx, pipe)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return []models.OrderPreview{}, nil
	}
	if err != nil {
		return
	}

	err = cur.All(ctx, &orders)
	if err != nil {
		return
	}

	return
}

func (r *CinemaOrdersRepository) GetOrderTotalPrice(ctx context.Context, orderId string) (price uint32, err error) {
	defer r.handleError(ctx, &err, "GetOrderTotalPrice")

	collection := r.db.Database(r.databaseName).Collection(ordersCollectionName)

	pipe := bson.A{
		bson.D{
			{Key: "$match",
				Value: bson.D{
					{Key: "order_id", Value: orderId},
				},
			},
		},
		bson.D{
			{Key: "$group",
				Value: bson.D{
					{Key: "_id", Value: "$order_id"},
					{Key: "total_price", Value: bson.D{{Key: "$sum", Value: "$price"}}},
				},
			},
		},
	}

	cur, err := collection.Aggregate(ctx, pipe)
	if err != nil {
		return
	}
	order := []struct {
		Id         string `bson:"_id"`
		TotalPrice uint32 `bson:"total_price"`
	}{}

	cur.All(ctx, &order)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return
	}
	if err != nil {
		r.logger.Errorf("error while getting order total price %v", err)
		return
	}
	if len(order) == 0 {
		return
	}

	return order[0].TotalPrice, nil
}

func (r *CinemaOrdersRepository) GetOrderItemsTotalPrice(ctx context.Context,
	orderId string, itemsIds []string) (total uint32, err error) {
	defer r.handleError(ctx, &err, "GetOrderItemsTotalPrice")

	collection := r.db.Database(r.databaseName).Collection(ordersCollectionName)

	var filter = bson.D{
		{Key: "order_id", Value: orderId},
	}

	cur, err := collection.Find(ctx, filter, options.Find().SetProjection(bson.D{
		{Key: "price", Value: 1},
		{Key: "_id", Value: 1},
	}))

	if err != nil {
		return
	}

	type orderItem struct {
		Id    string `bson:"_id"`
		Price uint32 `bson:"price"`
	}

	var items []orderItem
	err = cur.All(ctx, &items)
	if err != nil {
		return
	}

	if len(items) != len(itemsIds) {
		err = models.Error(models.NotFound, "items not found")
		return
	}

	for i := range items {
		total += items[i].Price
	}

	return
}

func (r *CinemaOrdersRepository) GetOrderScreeningId(ctx context.Context, accountId, orderId string) (id int64, err error) {
	defer r.handleError(ctx, &err, "GetOrderScreeningId")

	pipe := bson.A{
		bson.D{
			{Key: "$match",
				Value: bson.D{
					{Key: "order_id", Value: orderId},
					{Key: "owner_id", Value: accountId},
				},
			},
		},
		bson.D{
			{Key: "$group",
				Value: bson.D{
					{Key: "_id", Value: "$order_id"},
					{Key: "screening_id", Value: bson.D{{Key: "$first", Value: "$screening_id"}}},
				},
			},
		},
	}

	collection := r.db.Database(r.databaseName).Collection(ordersCollectionName)
	cur, err := collection.Aggregate(ctx, pipe)
	order := []struct {
		Id          string `bson:"_id"`
		ScreeningId int64  `bson:"screening_id"`
	}{}

	cur.All(ctx, &order)
	if err != nil {
		return
	}
	if len(order) == 0 {
		err = models.Error(models.NotFound, fmt.Sprintf("order with id %v not found", orderId))
		return
	}

	id = order[0].ScreeningId
	return
}

func (r *CinemaOrdersRepository) getOrderTickets(ctx context.Context, accountId string,
	orderId string) (tickets []models.Ticket, err error) {
	defer r.handleError(ctx, &err, "getOrderTickets")

	filter := bson.D{
		{Key: "owner_id", Value: accountId},
		{Key: "order_id", Value: orderId},
		{Key: "type", Value: ticketType},
	}

	collection := r.db.Database(r.databaseName).Collection(ordersCollectionName)
	cur, err := collection.Find(ctx, filter, options.Find().SetProjection(bson.D{
		{Key: "_id", Value: 1},
		{Key: "status", Value: 1},
		{Key: "place", Value: 1},
		{Key: "price", Value: 1},
	}))
	if err != nil {
		return
	}

	orderTickets := []struct {
		Id     string       `bson:"_id"`
		Status string       `bson:"status"`
		Place  models.Place `bson:"place"`
		Price  uint32       `bson:"price"`
	}{}

	cur.All(ctx, &orderTickets)
	if err != nil {
		return
	}

	tickets = make([]models.Ticket, 0, len(orderTickets))

	for i := range orderTickets {
		status, err := models.OrderItemStatusFromString(orderTickets[i].Status)
		// ignoring tickets with invalid status
		if err != nil {
			continue
		}
		tickets = append(tickets, models.Ticket{
			Id:     orderTickets[i].Id,
			Place:  orderTickets[i].Place,
			Price:  orderTickets[i].Price,
			Status: status,
		})
	}

	return

}

func (r *CinemaOrdersRepository) getOrderInfo(ctx context.Context,
	orderId, accountId string) (orderDate time.Time, ScreeningId int64, err error) {
	defer r.handleError(ctx, &err, "getOrderInfo")

	filter := bson.D{
		{Key: "owner_id", Value: accountId},
		{Key: "order_id", Value: orderId},
		{Key: "type", Value: ticketType},
	}
	collection := r.db.Database(r.databaseName).Collection(ordersCollectionName)
	res := collection.FindOne(ctx, filter, options.FindOne().SetProjection(bson.D{
		{Key: "order_date", Value: 1},
		{Key: "screening_id", Value: 1},
	}))

	err = res.Err()
	if err != nil {
		return
	}

	type order struct {
		Date        time.Time `bson:"order_date"`
		ScreeningId int64     `bson:"screening_id"`
	}

	ord := order{}
	err = res.Decode(&ord)
	if err != nil {
		return
	}

	return ord.Date, ord.ScreeningId, nil
}

func (r *CinemaOrdersRepository) GetOrder(ctx context.Context, orderId, accountId string) (res models.Order, err error) {
	defer r.handleError(ctx, &err, "GetOrder")
	r.logger.Info("order_id=", orderId, " account_id=", accountId)
	type orderInfoResp struct {
		orderDate   time.Time
		ScreeningId int64
		err         error
	}
	var orderInfoCh = make(chan orderInfoResp, 1)
	type orderTicketsResp struct {
		tickets []models.Ticket
		err     error
	}
	var orderTicketsCh = make(chan orderTicketsResp, 1)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		defer close(orderTicketsCh)

		res, err := r.getOrderTickets(ctx, accountId, orderId)
		orderTicketsCh <- orderTicketsResp{
			tickets: res,
			err:     err,
		}
	}()

	go func() {
		defer close(orderInfoCh)
		date, screeningId, err := r.getOrderInfo(ctx, orderId, accountId)
		orderInfoCh <- orderInfoResp{
			orderDate:   date,
			ScreeningId: screeningId,
			err:         err,
		}
	}()

	var infoReseived, ticketsReseived bool
	for !infoReseived || !ticketsReseived {
		select {
		case <-ctx.Done():
			return
		case info, ok := <-orderInfoCh:
			if !ok {
				continue
			}
			if info.err != nil {
				err = info.err
				return
			}
			res.Date = info.orderDate
			res.Id = orderId
			res.ScreeningId = info.ScreeningId
			infoReseived = true
		case ticketsRes, ok := <-orderTicketsCh:
			if !ok {
				continue
			}
			if ticketsRes.err != nil {
				err = ticketsRes.err
				return
			}
			res.Tickets = ticketsRes.tickets
			ticketsReseived = true
		}
	}
	return
}

func convertSortDTOToOrderPreviewSortParams(sort models.SortDTO) (fieldname string, ordering int, err error) {
	switch {
	case strings.EqualFold("order_id", sort.FieldName) || strings.EqualFold("orderid", sort.FieldName):
		fieldname = "order_id"
	case strings.EqualFold("order_date", sort.FieldName) || strings.EqualFold("orderdate", sort.FieldName):
		fieldname = "order_date"
	case strings.EqualFold("total_price", sort.FieldName) || strings.EqualFold("totalprice", sort.FieldName):
		fieldname = "total_price"
	case strings.EqualFold("screenings_ids", sort.FieldName) || strings.EqualFold("screeningsids", sort.FieldName):
		fieldname = "screenings_ids"
	default:
		err = models.Error(models.InvalidArgument, "invalid sort fieldname")
		return
	}

	ordering = 1
	if sort.SortOrdering == models.DESC {
		ordering = -1
	}

	return
}

func (r *CinemaOrdersRepository) handleError(ctx context.Context, err *error, functionName string) {
	if ctx.Err() != nil {
		var code models.ErrorCode
		switch {
		case errors.Is(ctx.Err(), context.Canceled):
			code = models.Canceled
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			code = models.DeadlineExceeded
		}
		*err = models.Error(code, ctx.Err().Error())
		r.logError(*err, functionName)
		return
	}

	if err == nil || *err == nil {
		return
	}

	r.logError(*err, functionName)
	var repoErr = &models.ServiceError{}
	if !errors.As(*err, &repoErr) {
		var code models.ErrorCode
		switch {
		case errors.Is(*err, mongo.ErrNoDocuments):
			code = models.NotFound
			*err = models.Error(code, "")
		case *err != nil:
			code = models.Internal
			*err = models.Error(code, "repository iternal error")
		}

	}
}

func (r *CinemaOrdersRepository) logError(err error, functionName string) {
	if err == nil {
		return
	}

	var repoErr = &models.ServiceError{}
	if errors.As(err, &repoErr) {
		r.logger.WithFields(
			logrus.Fields{
				"error.function.name": functionName,
				"error.msg":           repoErr.Msg,
				"error.code":          repoErr.Code,
			},
		).Error("cinema orders repository error occurred")
	} else {
		r.logger.WithFields(
			logrus.Fields{
				"error.function.name": functionName,
				"error.msg":           err.Error(),
			},
		).Error("cinema orders repository error occurred")
	}
}
