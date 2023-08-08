package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	binancesdk "github.com/adshao/go-binance/v2"
	"github.com/isther/arbitrage-htc/utils"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var (
	klineRatioBase = decimal.NewFromInt(10000)
)

type BookTickerEventUpdater interface {
	UpdateBookTickerEvent(event *binancesdk.WsBookTickerEvent)
}

type ArbitrageManager struct {
	BookTickerASymbol string
	StableCoinSymbol  string
	BookTickerBSymbol string

	updateEvents []BookTickerEventUpdater
	lock         sync.RWMutex
}

func NewArbitrageManager(
	bookTickerASymbol,
	stableCoinSymbol,
	bookTickerBSymbol string,
) *ArbitrageManager {
	return &ArbitrageManager{
		BookTickerASymbol: bookTickerASymbol,
		StableCoinSymbol:  stableCoinSymbol,
		BookTickerBSymbol: bookTickerBSymbol,
		updateEvents:      make([]BookTickerEventUpdater, 0),
		lock:              sync.RWMutex{},
	}
}

func (b *ArbitrageManager) Run() {
	go b.ping()
	go b.startBinanceBookTickerWebsocket()
	go b.startCheckBinanceKline()
}

// Get binance book ticker
func (b *ArbitrageManager) startBinanceBookTickerWebsocket() {
	b.lock.RLock()
	defer b.lock.RUnlock()

	handler := func(event *binancesdk.WsBookTickerEvent) {
		b.lock.RLock()
		defer b.lock.RUnlock()

		for i := range b.updateEvents {
			go b.updateEvents[i].UpdateBookTickerEvent(event)
		}
	}
	errHandler := func(err error) {
		logrus.Error(err)
		b.startBinanceBookTickerWebsocket()
	}

	doneC, stopC, err := binancesdk.WsCombinedBookTickerServe(
		[]string{b.BookTickerASymbol, b.StableCoinSymbol, b.BookTickerBSymbol},
		handler,
		errHandler,
	)
	if err != nil {
		logrus.Error(err)
		b.startBinanceBookTickerWebsocket()
	}
	logrus.Info("Connect to binance bookticker websocket server successfully.")

	_ = doneC
	_ = stopC
}

// Check binance K-line amplitude
func (b *ArbitrageManager) startCheckBinanceKline() {
	b.lock.RLock()
	defer b.lock.RUnlock()

	handler := func(event *binancesdk.WsKlineEvent) {
		high, _ := decimal.NewFromString(event.Kline.High)
		low, _ := decimal.NewFromString(event.Kline.Low)

		ratio := high.Div(low).Sub(decimal.NewFromInt(1)).Mul(klineRatioBase)

		if ratio.GreaterThan(viper.Get("PauseMaxKlineRatio").(decimal.Decimal)) {
			klinePauser.Pause(fmt.Sprintf("[Pause[] %s: The amplitude of %s K-line to high.", ratio.String(), b.BookTickerBSymbol))
		} else if ratio.LessThan(viper.Get("PauseMinKlineRatio").(decimal.Decimal)) {
			klinePauser.Pause(fmt.Sprintf("[Pause[] %s: The amplitude of %s K-line to low.", ratio.String(), b.BookTickerBSymbol))
		} else {
			klinePauser.UnPause(fmt.Sprintf("[Unpause[] %s: The amplitude of %s K-line recovery.", ratio.String(), b.BookTickerBSymbol))
		}
	}

	errHandler := func(err error) {
		if err != nil {
			logrus.WithFields(logrus.Fields{"server": "K-line"}).Error(err.Error())
			b.startCheckBinanceKline()
		}
	}

	doneC, stopC, err := binancesdk.WsKlineServe(b.BookTickerBSymbol, "1m", handler, errHandler)
	if err != nil {
		logrus.Error(err)
		b.startCheckBinanceKline()
	}
	logrus.Info("Connect to binance kline websocket server successfully.")

	_ = doneC
	_ = stopC

}

// Ping
func (b *ArbitrageManager) ping() {
	for {
		serverTime, err := utils.NewBinanceClient().NewServerTimeService().Do(context.Background())
		if err != nil {
			logrus.Errorf("Failed to get server time: %v", err.Error())
			continue
		}

		timeout := time.Now().UTC().UnixMilli() - serverTime

		if timeout > viper.GetInt64("PauseClientTimeOutLimit") {
			timeoutPauser.Pause(fmt.Sprintf("[Pause[] %dms: Binance timeout", timeout))
		} else {
			timeoutPauser.UnPause(fmt.Sprintf("[Unpause[] %dms: Binance timeout recovery", timeout))
		}
		time.Sleep(1 * time.Second)
	}
}

func (b *ArbitrageManager) AddUpdateEvent(event BookTickerEventUpdater) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.updateEvents = append(b.updateEvents, event)
}
