package core

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	binancesdk "github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/futures"
	"github.com/isther/arbitrage-htc/account"
	"github.com/isther/arbitrage-htc/utils"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
)

/*
							==> UNFILLED										==> END
INIT	==>	PROCESSOPEN		==> FILLED		==> PROCESSCLOSE	==> FILLED		==> END
																==> UNFILLED	==> END
*/

var ratioBase = decimal.NewFromInt(10000)

type TaskStatus int

var (
	INIT         TaskStatus = 0
	PROCESSOPEN  TaskStatus = 1
	PROCESSCLOSE TaskStatus = 2
	FILLED       TaskStatus = 3
	UNFILLED     TaskStatus = 4
)

func (t TaskStatus) String() string {
	switch t {
	case INIT:
		return "INIT"
	case PROCESSOPEN:
		return "PROCESSOPEN"
	case PROCESSCLOSE:
		return "PROCESSCLOSE"
	case FILLED:
		return "FILLED"
	case UNFILLED:
		return "UNFILLED"
	default:
		return "UNKNOWN"
	}
}

type TaskInfo interface {
	TaskInfo() string
}

func (t *Task) TaskInfo() string {
	return fmt.Sprintf(
		"Status:%v|Progress:%v/%v|TradeSymbol:%v|Qty:%v|Profit:%v|Mode1:%v|Mode2:%v|MinRatio:%v|MaxRatio:%v|ProfitRatio:%v|isFOK:%v|isFuture:%v|WaitDuration:%v|CloseTimeOut:%v\n",
		t.status,
		t.completedCnt,
		*t.cycleNumber,
		t.bookTickerBSymbol,
		*t.maxQty,
		t.profit.String(),
		t.mode1Ratio.String(),
		t.mode2Ratio.String(),
		t.minRatio,
		t.maxRatio,
		t.profitRatio,
		*t.isFOK,
		*t.isFuture,
		*t.waitDuration,
		*t.closeTimeOut,
	)
}

type Task struct {
	account.ExchangeInfo
	account.Balance
	account.OrderList

	status       TaskStatus
	completedCnt int
	mode1Ratio   decimal.Decimal
	mode2Ratio   decimal.Decimal

	isFOK             *bool
	isFuture          *bool
	onlyMode1         *bool
	maxQty            *string
	cycleNumber       *int
	waitDuration      *int64 // ms
	closeTimeOut      *int64 // ms
	minRatio          decimal.Decimal
	maxRatio          decimal.Decimal
	profitRatio       decimal.Decimal
	bookTickerASymbol string
	stableCoinSymbol  string
	bookTickerBSymbol string

	bookTickerASymbolEvent *binancesdk.WsBookTickerEvent
	stableCoinSymbolEvent  *binancesdk.WsBookTickerEvent
	bookTickerBSymbolEvent *binancesdk.WsBookTickerEvent

	closeRatio                 decimal.Decimal
	openBookTickerASymbolPrice decimal.Decimal
	openStableCoinPrice        decimal.Decimal
	openBookTickerBSymbolPrice decimal.Decimal

	profit decimal.Decimal

	openID  string
	closeID string

	doCh   chan struct{}
	stopCh chan struct{}

	mode atomic.Int32
	lock sync.RWMutex
}

func NewTask(
	binanceApiKey,
	binanceSecretKey string,
	isFOK, isFuture, OnlyMode1 *bool,
	maxQty *string,
	cycleNumber *int,
	waitDuration, closeTimeOut *int64,
	ratio, minRatio, maxRatio float64,
	bookTickerASymbol, stableCoinSymbol, bookTickerBSymbol string,
	exchangeInfo account.ExchangeInfo,
	balanceUpdate account.Balance,
	orderUpdate account.OrderList,
) *Task {
	return &Task{
		status:       INIT,
		ExchangeInfo: exchangeInfo,
		Balance:      balanceUpdate,
		OrderList:    orderUpdate,

		mode1Ratio: decimal.Zero,
		mode2Ratio: decimal.Zero,

		isFOK:             isFOK,
		isFuture:          isFuture,
		onlyMode1:         OnlyMode1,
		maxQty:            maxQty,
		cycleNumber:       cycleNumber,
		waitDuration:      waitDuration,
		closeTimeOut:      closeTimeOut,
		profitRatio:       decimal.NewFromFloat(ratio),
		minRatio:          decimal.NewFromFloat(minRatio).Div(ratioBase),
		maxRatio:          decimal.NewFromFloat(maxRatio).Div(ratioBase),
		bookTickerASymbol: strings.ToUpper(bookTickerASymbol),
		stableCoinSymbol:  strings.ToUpper(stableCoinSymbol),
		bookTickerBSymbol: strings.ToUpper(bookTickerBSymbol),

		profit: decimal.Zero,

		doCh:   make(chan struct{}),
		stopCh: make(chan struct{}),
	}
}

func (t *Task) UpdateBookTickerEvent(event *binancesdk.WsBookTickerEvent) {
	switch event.Symbol {
	case t.bookTickerASymbol:
		t.updateBookTickerASymbolEvent(event)
	case t.stableCoinSymbol:
		t.updateStableCoinSymbolEvent(event)
	case t.bookTickerBSymbol:
		t.updateBookTickerBSymbolEvent(event)
	}

	t.doCh <- struct{}{}
}

func (t *Task) updateBookTickerASymbolEvent(event *binancesdk.WsBookTickerEvent) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.bookTickerASymbolEvent = event
}

func (t *Task) updateStableCoinSymbolEvent(event *binancesdk.WsBookTickerEvent) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.stableCoinSymbolEvent = event
}

func (t *Task) updateBookTickerBSymbolEvent(event *binancesdk.WsBookTickerEvent) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.bookTickerBSymbolEvent = event
}

func (t *Task) Run() {
	// Init
	t.mode.Store(0)

	for {
		select {
		case <-t.doCh:
			t.trade()
		case <-t.stopCh:
			logrus.Info("Stop")
		}
	}

}

func (t *Task) Init() {
	t.mode.Store(0)
	t.status = INIT
	time.Sleep(time.Duration(*t.waitDuration) * time.Millisecond)
}

func (t *Task) completeTask() {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.completedCnt++
	logrus.Infof("Task progress: %d/%d", t.completedCnt, *t.cycleNumber)
	t.Balance.BalanceUpdate()
	if *t.cycleNumber == t.completedCnt {
		logrus.Info("Task completed", t.completedCnt)
		time.Sleep(time.Second * 4)
		panic("Task completed, exit")
	}
}

func (t *Task) trade() {
	if t.bookTickerASymbolEvent == nil || t.stableCoinSymbolEvent == nil || t.bookTickerBSymbolEvent == nil {
		return
	}

	if t.status == INIT {
		// Check and Open
		if !klinePauser.Value() && !timeoutPauser.Value() {
			t.status = PROCESSOPEN
			t.closeRatio, t.openStableCoinPrice, t.openBookTickerASymbolPrice, t.openBookTickerBSymbolPrice,
				t.openID = t.open()
			if t.status == FILLED {
				t.Balance.BalanceUpdate()
			}
		}
	} else {
		if t.status == FILLED ||
			(t.status == UNFILLED && !*t.isFOK) {
			time.Sleep(100 * time.Millisecond)

			// Close
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*t.closeTimeOut)*time.Millisecond)
			defer cancel()

			t.closeID = t.close(ctx)

			go func(mode int32, openID, closeID string) {
				openOrder, closeOrder := t.OrderList.OrderIDsUpdate(&account.OrderIDs{
					Mode:         mode,
					OpenOrderID:  openID,
					CloseOrderID: closeID,
				})

				var profit decimal.Decimal
				switch mode {
				case 1:
					profit = closeOrder.Price.Sub(openOrder.Price)
				case 2:
					profit = openOrder.Price.Sub(closeOrder.Price)
				default:
					panic("Invalid mode")
				}

				logrus.Info(fmt.Sprintf("Mode%d \n[Open]: BTC/USDT: %s\n[Close]: BTC/USDT: %s\n[Actual profit] BTC/USDT: %s",
					mode,
					openOrder.Price.String(),
					closeOrder.Price.String(),
					profit.String(),
				))

				t.lock.Lock()
				defer t.lock.Unlock()
				t.profit = t.profit.Add(profit)

			}(t.mode.Load(), t.openID, t.closeID)
			t.completeTask()
			return
		}
		t.Init()
	}
}

// Open
func (t *Task) open() (decimal.Decimal, decimal.Decimal, decimal.Decimal, decimal.Decimal, string) {
	ratio, stableSymbolPrice, bookTickerASymbolPrice, bookTickerBSymbolPrice, tradeID := t.openMode1()
	switch t.status {
	case FILLED:
		return ratio, stableSymbolPrice, bookTickerASymbolPrice, bookTickerBSymbolPrice, tradeID
	case UNFILLED:
		return decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, ""
	}
	if *t.onlyMode1 {
		t.status = INIT
		return decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, ""
	}

	ratio, stableSymbolPrice, bookTickerASymbolPrice, bookTickerBSymbolPrice, tradeID = t.openMode2()
	switch t.status {
	case FILLED:
		return ratio, stableSymbolPrice, bookTickerASymbolPrice, bookTickerBSymbolPrice, tradeID
	case UNFILLED:
		return decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, ""
	}
	return decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, ""

}

// Mode1
func (t *Task) openMode1() (decimal.Decimal, decimal.Decimal, decimal.Decimal, decimal.Decimal, string) {
	// Prepare price
	t.lock.RLock()
	stableSymbolBidPrice, _ := decimal.NewFromString(t.stableCoinSymbolEvent.BestBidPrice)
	bookTickerBSymbolAskPrice, _ := decimal.NewFromString(t.bookTickerBSymbolEvent.BestAskPrice)
	bookTickerASymbolBidPrice, _ := decimal.NewFromString(t.bookTickerASymbolEvent.BestBidPrice)
	t.lock.RUnlock()

	ratioMode1, ok := t.calculateRatioMode1(bookTickerASymbolBidPrice, bookTickerBSymbolAskPrice, stableSymbolBidPrice)
	if !ok {
		t.status = INIT
		return decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, ""
	}

	if ratioMode1.GreaterThanOrEqual(t.minRatio) && ratioMode1.LessThanOrEqual(t.maxRatio) {
		t.mode.Store(1)
		logrus.Info("[Open[]", t.ratioLog(ratioMode1, stableSymbolBidPrice, bookTickerASymbolBidPrice, bookTickerBSymbolAskPrice))
		price := utils.StringToDecimal(bookTickerBSymbolAskPrice.String()).Add(decimal.NewFromInt(1))
		if id, ok := t.tradeMode1(
			true,
			price,
			*t.maxQty,
		); ok {
			t.lock.RLock()
			stableSymbolAskPrice, _ := decimal.NewFromString(t.stableCoinSymbolEvent.BestAskPrice)
			t.lock.RUnlock()

			t.status = FILLED
			return ratioMode1, stableSymbolAskPrice, bookTickerASymbolBidPrice, bookTickerBSymbolAskPrice, id
		} else {
			t.status = UNFILLED
			return decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, ""
		}
	}

	t.status = INIT
	return decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, ""
}

// Mode2
func (t *Task) openMode2() (decimal.Decimal, decimal.Decimal, decimal.Decimal, decimal.Decimal, string) {
	// Prepare price
	t.lock.RLock()
	stableSymbolAskPrice, _ := decimal.NewFromString(t.stableCoinSymbolEvent.BestAskPrice)
	bookTickerASymbolAskPrice, _ := decimal.NewFromString(t.bookTickerASymbolEvent.BestAskPrice)
	bookTickerBSymbolBidPrice, _ := decimal.NewFromString(t.bookTickerBSymbolEvent.BestBidPrice)
	t.lock.RUnlock()

	ratioMode2, ok := t.calculateRatioMode2(bookTickerASymbolAskPrice, bookTickerBSymbolBidPrice, stableSymbolAskPrice)
	if !ok {
		t.status = INIT
		return decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, ""
	}

	if ratioMode2.GreaterThanOrEqual(t.minRatio) && ratioMode2.LessThanOrEqual(t.maxRatio) {
		t.mode.Store(2)
		logrus.Info("[Open[]", t.ratioLog(ratioMode2, stableSymbolAskPrice, bookTickerASymbolAskPrice, bookTickerBSymbolBidPrice))
		price := utils.StringToDecimal(bookTickerBSymbolBidPrice.String()).Sub(decimal.NewFromInt(1))
		if id, ok := t.tradeMode2(
			true,
			price,
			*t.maxQty,
		); ok {
			t.lock.RLock()
			stableSymbolBidPrice, _ := decimal.NewFromString(t.stableCoinSymbolEvent.BestBidPrice)
			t.lock.RUnlock()

			t.status = FILLED
			return ratioMode2, stableSymbolBidPrice, bookTickerASymbolAskPrice, bookTickerBSymbolBidPrice, id
		} else {
			t.status = UNFILLED
			return decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, ""
		}
	}

	t.status = INIT
	return decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, ""
}

// Close
func (t *Task) close(
	ctx context.Context,
) string {
	t.status = PROCESSCLOSE
	for {
		select {
		case <-ctx.Done():
			return t.foreceClose()
		default:
			switch t.mode.Load() {
			case 1:
				bookTickerBSymbolAskPrice, _ := decimal.NewFromString(t.bookTickerBSymbolEvent.BestAskPrice)
				bookTickerBSymbolBidPrice, _ := decimal.NewFromString(t.bookTickerBSymbolEvent.BestBidPrice)
				if t.openBookTickerBSymbolPrice.Sub(bookTickerBSymbolAskPrice).Cmp(decimal.NewFromFloat(1.0)) > 0 {
					if id, ok := t.tradeMode2(
						false,
						bookTickerBSymbolBidPrice,
						*t.maxQty,
					); ok {

						logrus.Infof("Mode1 loss after open, force close. Open price: %s, ask: %s, amount of loss: %s",
							t.openBookTickerBSymbolPrice.String(),
							bookTickerBSymbolAskPrice.String(),
							t.openBookTickerBSymbolPrice.Sub(bookTickerBSymbolAskPrice).String(),
						)
						t.expectProfitLog(bookTickerBSymbolAskPrice)
						return id
					}
				}

				// Do mode 2
				ratio := decimal.NewFromFloat(-0.0001).Sub(t.closeRatio).Mul(t.profitRatio)
				bookTickerASymbolAskPrice, _ := decimal.NewFromString(t.bookTickerASymbolEvent.BestAskPrice)
				ratioMode2, ok := t.calculateRatioMode2(bookTickerASymbolAskPrice, bookTickerBSymbolBidPrice, t.openStableCoinPrice)
				if !ok {
					continue
				}

				if ratioMode2.GreaterThanOrEqual(ratio) {
					logrus.Info("[Close[]", t.ratioLog(ratioMode2, t.openStableCoinPrice, bookTickerASymbolAskPrice, bookTickerBSymbolBidPrice))
					t.expectProfitLog(bookTickerBSymbolBidPrice)

					// Trade
					if id, ok := t.tradeMode2(
						false,
						bookTickerBSymbolBidPrice,
						*t.maxQty,
					); ok {
						return id
					}
				}
			case 2:
				bookTickerBSymbolBidPrice, _ := decimal.NewFromString(t.bookTickerBSymbolEvent.BestBidPrice)
				bookTickerBSymbolAskPrice, _ := decimal.NewFromString(t.bookTickerBSymbolEvent.BestAskPrice)
				if t.openBookTickerBSymbolPrice.Sub(bookTickerBSymbolBidPrice).Cmp(decimal.NewFromFloat(-1.0)) < 0 {
					if id, ok := t.tradeMode1(
						false,
						bookTickerBSymbolAskPrice,
						*t.maxQty,
					); ok {
						logrus.Infof("Mode2 loss after open, force close. Open price: %s, bid: %s, amount of loss: %s",
							t.openBookTickerBSymbolPrice.String(),
							bookTickerBSymbolBidPrice.String(),
							t.openBookTickerBSymbolPrice.Sub(bookTickerBSymbolBidPrice).String(),
						)
						t.expectProfitLog(bookTickerBSymbolBidPrice)
						return id
					}
				}

				// 做模式1
				ratio := decimal.NewFromFloat(-0.0001).Sub(t.closeRatio).Mul(t.profitRatio)
				bookTickerASymbolBidPrice, _ := decimal.NewFromString(t.bookTickerASymbolEvent.BestBidPrice)
				ratioMode1, ok := t.calculateRatioMode1(bookTickerASymbolBidPrice, bookTickerBSymbolAskPrice, t.openStableCoinPrice)
				if !ok {
					continue
				}

				if ratioMode1.GreaterThanOrEqual(ratio) {
					logrus.Info("[Close[]:", t.ratioLog(ratioMode1, t.openStableCoinPrice, bookTickerASymbolBidPrice, bookTickerBSymbolAskPrice))
					t.expectProfitLog(bookTickerBSymbolAskPrice)

					// Trade
					if id, ok := t.tradeMode1(
						false,
						bookTickerBSymbolAskPrice,
						*t.maxQty,
					); ok {
						return id
					}
				}
			}
		}
	}
}

func (t *Task) foreceClose() (orderID string) {
	switch t.mode.Load() {
	case 1:
		// Get Price
		bookTickerASymbolAskPrice, _ := decimal.NewFromString(t.bookTickerASymbolEvent.BestAskPrice)
		bookTickerBSymbolBidPrice, _ := decimal.NewFromString(t.bookTickerBSymbolEvent.BestBidPrice)

		// Log
		ratio, _ := t.calculateRatioMode2(bookTickerASymbolAskPrice, bookTickerBSymbolBidPrice, t.openStableCoinPrice)
		logrus.Info("[Force close[]",
			t.ratioLog(
				ratio,
				t.openStableCoinPrice,
				bookTickerASymbolAskPrice,
				bookTickerBSymbolBidPrice,
			),
		)
		t.expectProfitLog(bookTickerBSymbolBidPrice)

		if id, ok := t.tradeMode2(
			false,
			bookTickerBSymbolBidPrice,
			*t.maxQty,
		); ok {
			return id
		}
	case 2:
		// Get Price
		bookTickerBSymbolBidPrice, _ := decimal.NewFromString(t.bookTickerASymbolEvent.BestAskPrice)
		bookTickerBSymbolAskPrice, _ := decimal.NewFromString(t.bookTickerBSymbolEvent.BestBidPrice)

		// Log
		ratio, _ := t.calculateRatioMode1(bookTickerBSymbolBidPrice, bookTickerBSymbolAskPrice, t.openStableCoinPrice)
		logrus.Info("[Force close[]",
			t.ratioLog(
				ratio,
				t.openStableCoinPrice,
				bookTickerBSymbolBidPrice,
				bookTickerBSymbolAskPrice,
			),
		)
		t.expectProfitLog(bookTickerBSymbolAskPrice)

		if id, ok := t.tradeMode1(
			false,
			bookTickerBSymbolAskPrice,
			*t.maxQty,
		); ok {
			return id
		}
	}
	return orderID
}

func (t *Task) tradeMode1(
	isOpen bool,
	price decimal.Decimal,
	qty string,
) (string, bool) {
	if isOpen && *t.isFOK {
		if *t.isFuture {
			return t.binanceFuturesFOKTrade(
				t.bookTickerBSymbol,
				futures.SideTypeBuy,
				price.String(),
				qty,
			)
		} else {
			return t.binanceFOKTrade(
				t.bookTickerBSymbol,
				binancesdk.SideTypeBuy,
				price.String(),
				qty,
			)
		}
	}

	if *t.isFuture {
		return t.binanceFuturesTrade(
			t.bookTickerBSymbol,
			futures.SideTypeBuy,
			qty,
		)
	} else {
		return t.binanceTrade(
			t.bookTickerBSymbol,
			binancesdk.SideTypeBuy,
			qty,
		)
	}
}

func (t *Task) tradeMode2(
	isOpen bool,
	price decimal.Decimal,
	qty string,
) (string, bool) {
	if isOpen && *t.isFOK {
		if *t.isFuture {
			return t.binanceFuturesFOKTrade(
				t.bookTickerBSymbol,
				futures.SideTypeSell,
				price.String(),
				qty,
			)
		} else {
			return t.binanceFOKTrade(
				t.bookTickerBSymbol,
				binancesdk.SideTypeSell,
				price.String(),
				qty,
			)
		}
	}

	if *t.isFuture {
		return t.binanceFuturesTrade(
			t.bookTickerBSymbol,
			futures.SideTypeSell,
			qty,
		)
	} else {
		return t.binanceTrade(
			t.bookTickerBSymbol,
			binancesdk.SideTypeSell,
			qty,
		)
	}
}

func (t *Task) calculateRatioMode1(taPrice, tbPrice, stableSymbolPrice decimal.Decimal) (decimal.Decimal, bool) {
	t.lock.RLock()
	defer t.lock.RUnlock()

	if taPrice.IsZero() || tbPrice.IsZero() || stableSymbolPrice.IsZero() {
		return decimal.Zero, false
	}

	t.mode1Ratio = stableSymbolPrice.
		Sub(
			tbPrice.Div(taPrice),
		)
	return t.mode1Ratio, true
}

func (t *Task) calculateRatioMode2(taPrice, tbPrice, stableSymbolPrice decimal.Decimal) (decimal.Decimal, bool) {
	t.lock.RLock()
	defer t.lock.RUnlock()

	if taPrice.IsZero() || tbPrice.IsZero() || stableSymbolPrice.IsZero() {
		return decimal.Zero, false
	}

	t.mode2Ratio = decimal.NewFromFloat32(1).Div(stableSymbolPrice).
		Sub(
			taPrice.Div(
				tbPrice,
			),
		)
	return t.mode2Ratio, true
}

func (t *Task) ratioLog(ratio, stableSymbolPrice, taPrice, tbPrice decimal.Decimal) string {
	return fmt.Sprintf(
		"Status: %s [Mode%d] BTC/USDT: %s Ratio: %s",
		t.status,
		t.mode.Load(),
		tbPrice,
		ratio.Mul(decimal.NewFromFloat(10000)).String(),
	)
}

func (t *Task) expectProfitLog(closeBookTickerBSymbolPrice decimal.Decimal) {
	var (
		profit = decimal.Zero
	)

	switch t.mode.Load() {
	case 1:
		profit = closeBookTickerBSymbolPrice.Sub(t.openBookTickerBSymbolPrice)
	case 2:
		profit = t.openBookTickerBSymbolPrice.Sub(closeBookTickerBSymbolPrice)
	default:
		panic("Invalid mode")
	}

	msg := fmt.Sprintf("Mode%d \n[Open[]: BTC/USDT: %s\n[Close[]: BTC/USDT: %s\n[Expect profit[]: %s",
		t.mode.Load(),
		t.openBookTickerBSymbolPrice.String(),
		closeBookTickerBSymbolPrice.String(),
		profit.String(),
	)

	logrus.Infof(msg)
}

func (t *Task) actualProfitLog(closeBookTickerBSymbolPrice decimal.Decimal) {
	var (
		profit = decimal.Zero
	)

	switch t.mode.Load() {
	case 1:
		profit = closeBookTickerBSymbolPrice.Sub(t.openBookTickerBSymbolPrice)
	case 2:
		profit = t.openBookTickerBSymbolPrice.Sub(closeBookTickerBSymbolPrice)
	default:
		panic("Invalid mode")
	}

	msg := fmt.Sprintf("Mode%d \n[Open[]: BTC/USDT: %s\n[Close[]: BTC/USDT: %s\n[Expect profit[]: %s",
		t.mode.Load(),
		t.openBookTickerBSymbolPrice.String(),
		closeBookTickerBSymbolPrice.String(),
		profit.String(),
	)

	logrus.Infof(msg)
}

func (t *Task) binanceFOKTrade(symbol string, side binancesdk.SideType, price, qty string) (string, bool) {
	quantityDecimal, _ := decimal.NewFromString(qty)
	quantityDecimal = quantityDecimal.Truncate(5).Truncate(8)
	qty = quantityDecimal.String()

	res, err := utils.NewBinanceClient().NewCreateOrderService().
		Symbol(symbol).Side(side).TimeInForce(binancesdk.TimeInForceTypeFOK).
		Type(binancesdk.OrderTypeLimit).Price(price).Quantity(qty).
		Do(context.Background())
	if err != nil {
		logrus.Infof("Spot FOK Order --- Error: %v, %s", res, err)
		time.Sleep(time.Millisecond * 66)
		return "", false
	}

	switch res.Status {
	case binancesdk.OrderStatusTypeExpired:
		logrus.Info("Spot FOK Order --- Failed，Expired")
		time.Sleep(time.Millisecond * 66)
		return "", false
	case binancesdk.OrderStatusTypeFilled:
		return fmt.Sprintln(res.OrderID), true
	}
	return "", false
}

func (t *Task) binanceFuturesFOKTrade(symbol string, side futures.SideType, price, qty string) (string, bool) {
	quantityDecimal, _ := decimal.NewFromString(qty)
	quantityDecimal = quantityDecimal.Truncate(5).Truncate(8)
	qty = quantityDecimal.String()

	res, err := utils.NewBinanceFuturesClient().NewCreateOrderService().
		Symbol(symbol).Side(side).TimeInForce(futures.TimeInForceTypeFOK).
		Type(futures.OrderTypeLimit).Price(price).Quantity(qty).
		NewOrderResponseType(futures.NewOrderRespTypeRESULT).
		Do(context.Background())
	if err != nil {
		logrus.Infof("Future FOK Order --- Error: %v, %s", res, err.Error())
		time.Sleep(time.Millisecond * 66)
		return "", false
	}

	switch res.Status {
	case futures.OrderStatusTypeExpired:
		logrus.Info("Future FOK Order --- Failed，Expired")
		time.Sleep(time.Millisecond * 66)
		return "", false
	case futures.OrderStatusTypeFilled:
		return fmt.Sprintln(res.OrderID), true
	}
	return "", false
}

func (t *Task) binanceTrade(symbol string, side binancesdk.SideType, qty string) (string, bool) {
	t.CorrectionQty(symbol, utils.StringToDecimal(qty))

	res, err := utils.NewBinanceClient().NewCreateOrderService().
		Symbol(symbol).Side(side).Type(binancesdk.OrderTypeMarket).
		Quantity(qty).
		Do(context.Background())
	if err != nil {
		logrus.Error(res, err)
		return "", false
	}
	return fmt.Sprintln(res.OrderID), true
}

func (t *Task) binanceFuturesTrade(symbol string, side futures.SideType, qty string) (string, bool) {
	t.CorrectionQty(symbol, utils.StringToDecimal(qty))

	res, err := utils.NewBinanceFuturesClient().NewCreateOrderService().
		Symbol(symbol).Side(side).Type(futures.OrderTypeMarket).
		Quantity(qty).
		Do(context.Background())
	if err != nil {
		if strings.Contains(err.Error(), "code=-1001") { // Internal error
			// do
		}
		logrus.Error(res, err)
		return "", false
	}
	return fmt.Sprintln(res.OrderID), true
}
