package account

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	binancesdk "github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/futures"
	"github.com/isther/arbitrage-htc/utils"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
)

type OrderList interface {
	Run()
	OrderIDsUpdate(orderIDs *OrderIDs)
}

type ORDER struct {
	isFuture *bool

	binanceOrders   map[string]order
	binanceOrdersCh chan order

	L sync.RWMutex
}

type OrderIDs struct {
	Mode         int32
	OpenOrderID  string
	CloseOrderID string
}

type order struct {
	ID    string
	Price decimal.Decimal
	Qty   decimal.Decimal
}

func NewORDER(isFuture *bool) *ORDER {
	return &ORDER{
		isFuture:        isFuture,
		binanceOrders:   make(map[string]order),
		binanceOrdersCh: make(chan order),
		L:               sync.RWMutex{},
	}
}

func (o *ORDER) Run() {
	go func() {
		for {
			order := <-o.binanceOrdersCh
			o.L.Lock()
			o.binanceOrders[order.ID] = order
			o.L.Unlock()
		}
	}()

	if *o.isFuture {
		o.startBinanceFuturesAccountServer()
	} else {
		o.startBinanceAccountServer()
	}
}

func (o *ORDER) OrderIDsUpdate(orderIDs *OrderIDs) {
	o.profitLog(orderIDs)
}

func (o *ORDER) startBinanceAccountServer() {
	listenKey, err := utils.NewBinanceClient().NewStartUserStreamService().Do(context.Background())
	if err != nil {
		o.startBinanceAccountServer()
	}
	// defer binance.CloseListenKey(binanceListenKey)

	go func() {
		for {
			time.Sleep(25 * time.Minute)
			utils.NewBinanceClient().NewKeepaliveUserStreamService().ListenKey(listenKey).Do(context.Background())
		}
	}()

	wsHandler := func(event *binancesdk.WsUserDataEvent) {
		switch event.Event {
		case binancesdk.UserDataEventTypeExecutionReport:
			o.orderUpdate(event.OrderUpdate)
		}

	}
	errHandler := func(err error) {
		if err != nil {
			logrus.Error(err.Error())
			o.startBinanceAccountServer()
		}
	}

	doneC, stopC, err := binancesdk.WsUserDataServe(listenKey, wsHandler, errHandler)
	if err != nil {
		logrus.Error(err.Error())
		o.startBinanceAccountServer()
	}
	logrus.Info("Connect to binance account websocket server successfully.")

	_ = doneC
	_ = stopC
}

func (o *ORDER) startBinanceFuturesAccountServer() {
	listenKey, err := utils.NewBinanceFuturesClient().NewStartUserStreamService().Do(context.Background())
	if err != nil {
		o.startBinanceFuturesAccountServer()
	}
	// defer binance.CloseListenKey(binanceListenKey)

	go func() {
		for {
			time.Sleep(25 * time.Minute)
			utils.NewBinanceFuturesClient().NewKeepaliveUserStreamService().ListenKey(listenKey).Do(context.Background())
		}
	}()

	wsHandler := func(event *futures.WsUserDataEvent) {
		switch event.Event {
		case futures.UserDataEventTypeOrderTradeUpdate:
			o.futuresOrderUpdate(event.OrderTradeUpdate)
		}

	}
	errHandler := func(err error) {
		if err != nil {
			logrus.Error(err.Error())
			o.startBinanceFuturesAccountServer()
		}
	}

	doneC, stopC, err := futures.WsUserDataServe(listenKey, wsHandler, errHandler)
	if err != nil {
		logrus.Error(err.Error())
		o.startBinanceAccountServer()
	}
	logrus.Info("Connect to binance futures account websocket server successfully.")

	_ = doneC
	_ = stopC
}

func (o *ORDER) orderUpdate(orderUpdate binancesdk.WsOrderUpdate) {
	switch orderUpdate.Status {
	case "NEW":
	case "CANCELED":
	case "FILLED":
		o.binanceOrdersCh <- order{
			ID:    fmt.Sprintln(orderUpdate.Id),
			Price: utils.StringToDecimal(orderUpdate.LatestPrice),
			Qty:   utils.StringToDecimal(orderUpdate.FilledVolume),
		}

	case "PARTIALLY_FILLED":
	default:
	}
}

func (o *ORDER) futuresOrderUpdate(orderUpdate futures.WsOrderTradeUpdate) {
	switch orderUpdate.Status {
	case "NEW":
	case "CANCELED":
	case "FILLED":
		o.binanceOrdersCh <- order{
			ID:    fmt.Sprintln(orderUpdate.ID),
			Price: utils.StringToDecimal(orderUpdate.LastFilledPrice),
			Qty:   utils.StringToDecimal(orderUpdate.LastFilledQty),
		}

	case "PARTIALLY_FILLED":
	default:
	}
}

func (o *ORDER) profitLog(orderIDs *OrderIDs) {
	openBinanceOrder, closeBinanceOrder, ok := o.getOrders(orderIDs)
	if !ok {
		logrus.Info("Failed to get binance order")
		return
	}

	var (
		profit = decimal.Zero
		msg    string
	)

	switch orderIDs.Mode {
	case 1:
		profit = closeBinanceOrder.Price.Sub(openBinanceOrder.Price)
	case 2:
		profit = openBinanceOrder.Price.Sub(closeBinanceOrder.Price)
	default:
		panic("Invalid mode")
	}

	msg = fmt.Sprintf("Mode%d \n[Open]: BTC/USDT: %s\n[Close]: BTC/USDT: %s\n[Actual profit] BTC/USDT: %s",
		orderIDs.Mode,
		openBinanceOrder.Price.String(),
		closeBinanceOrder.Price.String(),
		profit.String(),
	)

	logrus.Infof(msg)
}

func (o *ORDER) getOrders(orderIds *OrderIDs) (openBinanceOrder, closeBinanceOrder order, ok bool) {
	var (
		wg          sync.WaitGroup
		ticker      = time.NewTicker(time.Millisecond * 10)
		ctx, cancel = context.WithTimeout(context.Background(), time.Millisecond*20000)
		cnt         atomic.Int64
	)
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()

		if openBinanceOrder, ok = o.getBinanceOrderWithContext(orderIds.OpenOrderID, ticker, ctx); ok {
			cnt.Add(1)
		} else {
			logrus.Info("Failed to get binance open order")
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		if closeBinanceOrder, ok = o.getBinanceOrderWithContext(orderIds.CloseOrderID, ticker, ctx); ok {
			cnt.Add(1)
		} else {
			logrus.Info("Failed to get binance close order")
		}
	}()

	wg.Wait()
	o.clearOrders()

	// if cnt.Load() != 4 {
	if cnt.Load() != 2 {
		return openBinanceOrder, closeBinanceOrder, false
	} else {
		return openBinanceOrder, closeBinanceOrder, true
	}

}

func (o *ORDER) clearOrders() {
	o.L.Lock()
	defer o.L.Unlock()

	o.binanceOrders = make(map[string]order)
}

func (o *ORDER) getBinanceOrderWithContext(id string, ticker *time.Ticker, ctx context.Context) (order, bool) {
	for {
		select {
		case <-ctx.Done():
			return order{}, false
		case <-ticker.C:
			if order, ok := o.getBinanceOrder(id); ok {
				return order, ok
			}
		}
	}
}

func (o *ORDER) getBinanceOrder(id string) (order, bool) {
	o.L.RLock()
	defer o.L.RUnlock()

	v, ok := o.binanceOrders[id]
	return v, ok
}
