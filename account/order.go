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
	"github.com/spf13/viper"
)

type OrderList interface {
	Run()
	OrderIDsUpdate(orderIDs *OrderIDs)
}

type ORDER struct {
	binanceOrders   map[string]OrderInfo
	binanceOrdersCh chan OrderInfo

	CntInputer

	L sync.RWMutex
}

type OrderIDs struct {
	Mode         int32
	OpenOrderID  string
	CloseOrderID string
}

type OrderInfo struct {
	ID    string
	Price decimal.Decimal
	Qty   decimal.Decimal
}

func NewORDER(cntInputer CntInputer) *ORDER {
	return &ORDER{
		binanceOrders:   make(map[string]OrderInfo),
		binanceOrdersCh: make(chan OrderInfo),
		CntInputer:      cntInputer,
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

	if viper.GetBool("Future") {
		o.startBinanceFuturesAccountServer()
	} else {
		o.startBinanceAccountServer()
	}
}

func (o *ORDER) OrderIDsUpdate(orderIDs *OrderIDs) {
	var (
		openOrder, closeOrder = o.getOrders(orderIDs)
		mode                  = orderIDs.Mode
	)
	var profit decimal.Decimal
	switch mode {
	case 1:
		profit = closeOrder.Price.Sub(openOrder.Price)
	case 2:
		profit = openOrder.Price.Sub(closeOrder.Price)
	default:
		panic("Invalid mode")
	}

	o.CntInputer.AddOrder(profit, time.Now())

	logrus.Info(fmt.Sprintf("Mode%d \n[Open]: BTC/USDT: %s\n[Close]: BTC/USDT: %s\n[Actual profit] BTC/USDT: %s",
		mode,
		openOrder.Price.String(),
		closeOrder.Price.String(),
		profit.String(),
	))
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
		o.binanceOrdersCh <- OrderInfo{
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
		o.binanceOrdersCh <- OrderInfo{
			ID:    fmt.Sprintln(orderUpdate.ID),
			Price: utils.StringToDecimal(orderUpdate.LastFilledPrice),
			Qty:   utils.StringToDecimal(orderUpdate.LastFilledQty),
		}

	case "PARTIALLY_FILLED":
	default:
	}
}

func (o *ORDER) getOrders(orderIds *OrderIDs) (*OrderInfo, *OrderInfo) {
	var (
		wg                                  sync.WaitGroup
		ticker                              = time.NewTicker(time.Millisecond * 10)
		ctx, cancel                         = context.WithTimeout(context.Background(), time.Millisecond*20000)
		cnt                                 atomic.Int64
		openBinanceOrder, closeBinanceOrder OrderInfo
	)
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()

		var ok bool
		if openBinanceOrder, ok = o.getBinanceOrderWithContext(orderIds.OpenOrderID, ticker, ctx); ok {
			cnt.Add(1)
		} else {
			logrus.Info("Failed to get binance open order")
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		var ok bool
		if closeBinanceOrder, ok = o.getBinanceOrderWithContext(orderIds.CloseOrderID, ticker, ctx); ok {
			cnt.Add(1)
		} else {
			logrus.Info("Failed to get binance close order")
		}
	}()
	wg.Wait()
	o.clearOrders()

	return &openBinanceOrder, &closeBinanceOrder
}

func (o *ORDER) clearOrders() {
	o.L.Lock()
	defer o.L.Unlock()

	o.binanceOrders = make(map[string]OrderInfo)
}

func (o *ORDER) getBinanceOrderWithContext(id string, ticker *time.Ticker, ctx context.Context) (OrderInfo, bool) {
	for {
		select {
		case <-ctx.Done():
			return OrderInfo{}, false
		case <-ticker.C:
			if order, ok := o.getBinanceOrder(id); ok {
				return order, ok
			}
		}
	}
}

func (o *ORDER) getBinanceOrder(id string) (OrderInfo, bool) {
	o.L.RLock()
	defer o.L.RUnlock()

	v, ok := o.binanceOrders[id]
	return v, ok
}
