package rediscache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Falokut/cinema_orders_service/internal/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
)

type ReserveCache struct {
	rdb    *redis.Client
	logger *logrus.Logger
}

func NewReserveCache(rdb *redis.Client, logger *logrus.Logger) *ReserveCache {
	return &ReserveCache{rdb: rdb, logger: logger}
}

func (c *ReserveCache) PingContext(ctx context.Context) error {
	if err := c.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("error while pinging reserve cache: %w", err)
	}
	return nil
}

func getKeyForPlace(screeningId int64, row, seat int32) string {
	return fmt.Sprintf("%d_%d_%d", screeningId, row, seat)
}
func parseKeyForPlace(key string) (screeningId int64, row, seat int32, err error) {
	parts := strings.Split(key, "_")
	if len(parts) != 3 {
		err = errors.New("invalid key")
		return
	}

	screeningId, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return
	}

	rowParsed, err := strconv.ParseInt(parts[1], 10, 32)
	if err != nil {
		return
	}
	row = int32(rowParsed)

	seatParsed, err := strconv.ParseInt(parts[2], 10, 32)
	if err != nil {
		return
	}
	seat = int32(seatParsed)

	return
}

type reservation struct {
	Places      []models.Place `json:"places"`
	ScreeningId int64          `json:"screening_id"`
}

func (c *ReserveCache) ReservePlaces(ctx context.Context,
	screeningId int64, places []models.Place, ttl time.Duration) (reservationId string, err error) {
	defer c.handleError(ctx, &err, "ReservePlaces")

	tx := c.rdb.Pipeline()
	toCache, err := json.Marshal(reservation{Places: places, ScreeningId: screeningId})
	if err != nil {
		return
	}

	reservationId = uuid.NewString()
	err = tx.Set(ctx, reservationId, toCache, ttl).Err()
	if err != nil {
		return
	}

	var keys = make([]string, len(places))
	for i, place := range places {
		key := getKeyForPlace(screeningId, place.Row, place.Seat)
		keys[i] = key

		err = tx.Set(ctx, key, key, ttl).Err()
		if err != nil {
			return
		}
	}

	err = tx.SAdd(ctx, fmt.Sprint(screeningId), keys).Err()
	if err != nil {
		return
	}

	_, err = tx.Exec(ctx)
	if err != nil {
		return
	}

	return
}

func (c *ReserveCache) GetReservation(ctx context.Context,
	reservationId string) (places []models.Place, screeningId int64, err error) {
	defer c.handleError(ctx, &err, "GetReservation")

	cached, err := c.rdb.Get(ctx, reservationId).Bytes()
	if err != nil {
		return
	}

	var reserv reservation
	err = json.Unmarshal(cached, &reserv)
	if err != nil {
		return
	}

	return reserv.Places, reserv.ScreeningId, nil
}

func (c *ReserveCache) DeletePlacesReservation(ctx context.Context, reservationId string) (err error) {
	defer c.handleError(ctx, &err, "DeletePlacesReservation")

	reservBody, err := c.rdb.Get(ctx, reservationId).Bytes()
	if err != nil {
		return
	}

	reserv := reservation{}
	err = json.Unmarshal(reservBody, &reserv)
	if err != nil {
		return
	}

	keys := make([]string, len(reserv.Places)+1)
	for i := range reserv.Places {
		keys[i] = getKeyForPlace(reserv.ScreeningId, reserv.Places[i].Row, reserv.Places[i].Seat)
	}
	keys[len(keys)-1] = reservationId
	tx := c.rdb.Pipeline()

	err = tx.Del(ctx, keys...).Err()
	if err != nil {
		return
	}
	err = c.rdb.SRem(ctx, fmt.Sprint(reserv.ScreeningId), keys[:len(keys)-1]).Err()
	if err != nil {
		return
	}
	_, err = tx.Exec(ctx)
	return
}

func (c *ReserveCache) removeNonexistantKeys(ctx context.Context,
	screeningId int64, keys []string) (existsKeys []string, err error) {
	defer c.handleError(ctx, &err, "removeNonexistantKeys")

	if len(keys) == 0 {
		return []string{}, nil
	}

	res, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return
	}

	var exists = make(map[string]struct{}, len(keys))
	for i := range res {
		if res[i] == nil {
			continue
		}
		exists[res[i].(string)] = struct{}{}
	}

	toRemoveLen := len(keys) - len(exists)
	if toRemoveLen == 0 {
		return keys, nil
	}

	var toRemove = make([]string, toRemoveLen)
	for i := range keys {
		if _, ok := exists[keys[i]]; !ok {
			toRemove = append(toRemove, keys[i])
		}
	}

	err = c.rdb.SRem(ctx, fmt.Sprint(screeningId), toRemove).Err()
	if err != nil {
		return
	}

	existsKeys = maps.Keys(exists)
	return
}

func (c *ReserveCache) GetReservedPlacesForScreening(ctx context.Context, screeningId int64) (places []models.Place, err error) {
	defer c.handleError(ctx, &err, "GetReservedPlacesForScreening")

	keys, err := c.rdb.SMembers(ctx, fmt.Sprint(screeningId)).Result()
	if err != nil {
		return
	}
	if len(keys) == 0 {
		return
	}

	keys, err = c.removeNonexistantKeys(ctx, screeningId, keys)
	if err != nil {
		return
	}

	places = make([]models.Place, 0, len(keys))
	for i := range keys {
		_, row, seat, _ := parseKeyForPlace(keys[i])
		places = append(places, models.Place{Row: row, Seat: seat})
	}

	return
}

func (c *ReserveCache) GetScreeningsReservedPlaces(ctx context.Context,
	ids []int64) (reservedPlaces map[int64][]models.Place, err error) {
	defer c.handleError(ctx, &err, "GetScreeningsReservedPlaces")

	reservedPlaces = make(map[int64][]models.Place, len(ids))
	for _, id := range ids {
		reserved, respErr := c.GetReservedPlacesForScreening(ctx, id)
		if err != nil {
			err = respErr
			return
		}
		reservedPlaces[id] = reserved
	}
	return
}

func (с *ReserveCache) handleError(ctx context.Context, err *error, functionName string) {
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

	с.logError(*err, functionName)
	var repoErr = &models.ServiceError{}
	if !errors.As(*err, &repoErr) {
		var code models.ErrorCode
		switch {
		case errors.Is(*err, redis.Nil):
			code = models.NotFound
			*err = models.Error(code, "enity not found")
		case err != nil:
			code = models.Internal
			*err = models.Error(code, "cache internal error")
		}
	}
}

func (c *ReserveCache) logError(err error, functionName string) {
	if err == nil {
		return
	}

	var repoErr = &models.ServiceError{}
	if errors.As(err, &repoErr) {
		c.logger.WithFields(
			logrus.Fields{
				"error.function.name": functionName,
				"error.msg":           repoErr.Msg,
				"error.code":          repoErr.Code,
			},
		).Error("reserve cache error occurred")
	} else {
		c.logger.WithFields(
			logrus.Fields{
				"error.function.name": functionName,
				"error.msg":           err.Error(),
				"error.code":          models.Unknown,
			},
		).Error("reserve cache error occurred")
	}
}
