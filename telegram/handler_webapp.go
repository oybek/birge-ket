package telegram

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/jellydator/ttlcache/v3"
	"github.com/oybek/choguuket/database"
	"github.com/oybek/choguuket/model"
)

func (lp *LongPoll) handleWebAppData(b *gotgbot.Bot, ctx *ext.Context) error {
	chat := &ctx.EffectiveMessage.Chat
	data := ctx.EffectiveMessage.WebAppData.Data

	log.Printf("Got webapp data: %s", data)

	if trip, err := parse[model.Trip](data); err == nil {
		return lp.handleNewTrip(chat, trip)
	}
	if tripReq, err := parse[model.TripReq](data); err == nil {
		return lp.handleNewTripReq(chat, tripReq)
	} else {
		log.Printf("error parsing: %s", err)
	}

	return nil
}

func (lp *LongPoll) handleNewTrip(chat *gotgbot.Chat, trip *model.Trip) error {
	_, err := database.Transact(lp.db, func(tx database.TransactionOps) (any, error) {
		return database.InsertTrip(tx, trip)
	})
	if err != nil {
		return fmt.Errorf("failed to insert trip: %w", err)
	}

	err = lp.sendText(chat, "Поездка создана ✅")
	if err != nil {
		return err
	}

	time.Sleep(300 * time.Millisecond)

	return lp.sendText(chat, trip.String())
}

func (lp *LongPoll) handleNewTripReq(chat *gotgbot.Chat, tripReq *model.TripReq) error {
	tripReqId, err := database.Transact(lp.db, func(tx database.TransactionOps) (int64, error) {
		return database.InsertTripReq(tx, tripReq)
	})
	if err != nil {
		return fmt.Errorf("failed to insert trip: %w", err)
	}

	err = lp.sendText(chat, "Ищу поездки по запросу:\n"+tripReq.String())
	if err != nil {
		return err
	}

	time.Sleep(time.Second)

	trips, err := database.Transact(lp.db, func(tx database.TransactionOps) ([]model.Trip, error) {
		return database.SearchTrip(tx, tripReq)
	})
	if err != nil {
		return err
	}

	if len(trips) == 0 {
		return lp.sendText(chat, "Пока нет поездок по Вашему запросу, как только появится поездка я Вам сообщу")
	}

	lp.searchCache.Set(tripReqId, trips, ttlcache.DefaultTTL)
	return lp.sendTrip(chat, tripReqId, trips)
}

func (lp *LongPoll) sendText(chat *gotgbot.Chat, text string) error {
	_, err := lp.bot.SendMessage(chat.Id, text, &gotgbot.SendMessageOpts{
		ParseMode: "markdown",
	})
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	return nil
}

func parse[T model.Validated](jsonRaw string) (*T, error) {
	var data T
	if err := json.Unmarshal([]byte(jsonRaw), &data); err != nil {
		return nil, err
	}
	if !data.IsValid() {
		return nil, errors.New("invalid data")
	}
	return &data, nil
}