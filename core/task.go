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
	"github.com/spf13/viper"
)

/*
							==> UNFILLED										==> END
INIT	==>	PROCESSOPEN		==> FILLED		==> PROCESSCLOSE	==> FILLED		==> END
																==> UNFILLED	==> END
*/

var ratioBase = decimal.NewFromInt(10000)

type TaskStatus int8

const (
	RUNNING TaskStatus = iota
	PROCESSOPEN
	PROCESSCLOSE
	FILLED
	UNFILLED
	PAUSE
	STOP
)

func (t TaskStatus) String() string {
	switch t {
	case RUNNING:
		return "RUNNING"
	case PROCESSOPEN:
		return "PROCESSOPEN"
	case PROCESSCLOSE:
		return "PROCESSCLOSE"
	case FILLED:
		return "FILLED"
	case UNFILLED:
		return "UNFILLED"
	case STOP:
		return "STOP"
	default:
		return "UNKNOWN"
	}
}

type Task struct {
	account.ExchangeInfo
	account.Balance
	account.OrderList

	status       TaskStatus
	mode         atomic.Int32
	completedCnt int
	gain         decimal.Decimal
	profit       decimal.Decimal
	deficit      decimal.Decimal

	mode1Ratio decimal.Decimal
	mode2Ratio decimal.Decimal

	bookTickerASymbol string
	stableCoinSymbol  string
	bookTickerBSymbol string

	maxQty       string
	cycleNumber  int
	waitDuration time.Duration
	closeTimeOut time.Duration
	fok          bool
	fokStandard  decimal.Decimal
	future       bool
	onlyMode1    bool
	ratioMin     decimal.Decimal
	ratioMax     decimal.Decimal
	ratioProfit  decimal.Decimal

	bookTickerASymbolAskPrice decimal.Decimal
	bookTickerASymbolBidPrice decimal.Decimal
	stableCoinSymbolAskPrice  decimal.Decimal
	stableCoinSymbolBidPrice  decimal.Decimal
	bookTickerBSymbolAskPrice decimal.Decimal
	bookTickerBSymbolBidPrice decimal.Decimal

	closeRatio                 decimal.Decimal
	openBookTickerASymbolPrice decimal.Decimal
	openStableCoinPrice        decimal.Decimal
	openBookTickerBSymbolPrice decimal.Decimal

	openID  string
	closeID string

	doCh   chan struct{}
	stopCh chan struct{}

	lock sync.RWMutex
}

func NewTask(
	binanceApiKey,
	binanceSecretKey string,
	// other
	bookTickerASymbol, stableCoinSymbol, bookTickerBSymbol string,
	exchangeInfo account.ExchangeInfo,
	balanceUpdate account.Balance,
	orderUpdate account.OrderList,
) *Task {
	return &Task{
		ExchangeInfo: exchangeInfo,
		Balance:      balanceUpdate,
		OrderList:    orderUpdate,

		status:       RUNNING,
		completedCnt: 0,
		gain:         decimal.Zero,
		profit:       decimal.Zero,
		deficit:      decimal.Zero,

		mode1Ratio: decimal.Zero,
		mode2Ratio: decimal.Zero,

		bookTickerASymbol: strings.ToUpper(bookTickerASymbol),
		stableCoinSymbol:  strings.ToUpper(stableCoinSymbol),
		bookTickerBSymbol: strings.ToUpper(bookTickerBSymbol),

		maxQty:       viper.GetString("MaxQty"),
		cycleNumber:  viper.GetInt("CycleNumber"),
		waitDuration: viper.GetDuration("WaitDuration"),
		closeTimeOut: viper.GetDuration("CloseTimeOut"),
		fok:          viper.GetBool("FOK"),
		fokStandard:  viper.Get("FOKStandard").(decimal.Decimal),
		future:       viper.GetBool("Future"),
		onlyMode1:    viper.GetBool("OnlyMode1"),
		ratioMin:     viper.Get("RatioMin").(decimal.Decimal),
		ratioMax:     viper.Get("RatioMax").(decimal.Decimal),
		ratioProfit:  viper.Get("RatioProfit").(decimal.Decimal),

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
	t.bookTickerASymbolAskPrice = utils.StringToDecimal(event.BestAskPrice)
	t.bookTickerASymbolBidPrice = utils.StringToDecimal(event.BestBidPrice)
}

func (t *Task) updateStableCoinSymbolEvent(event *binancesdk.WsBookTickerEvent) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.stableCoinSymbolAskPrice = utils.StringToDecimal(event.BestAskPrice)
	t.stableCoinSymbolBidPrice = utils.StringToDecimal(event.BestBidPrice)
}

func (t *Task) updateBookTickerBSymbolEvent(event *binancesdk.WsBookTickerEvent) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.bookTickerBSymbolAskPrice = utils.StringToDecimal(event.BestAskPrice)
	t.bookTickerBSymbolBidPrice = utils.StringToDecimal(event.BestBidPrice)
}

func (t *Task) Run() {
	// Init
	t.mode.Store(0)
	t.status = STOP
	t.UpdateBalance()

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
	t.status = RUNNING
	time.Sleep(t.waitDuration)
}

func (t *Task) completeTask() {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.completedCnt++
	t.UpdateBalance()
	if t.cycleNumber == t.completedCnt {
		logrus.Info("Task completed", t.completedCnt)
		t.status = STOP
		logrus.Info("Task completed, stop")
	}
}

func (t *Task) trade() {
	if t.bookTickerASymbolAskPrice.IsZero() || t.bookTickerBSymbolBidPrice.IsZero() ||
		t.stableCoinSymbolAskPrice.IsZero() || t.stableCoinSymbolBidPrice.IsZero() ||
		t.bookTickerBSymbolAskPrice.IsZero() || t.bookTickerBSymbolBidPrice.IsZero() {
		return
	}

	if t.status == STOP {
		return
	}

	if t.status == RUNNING {
		// Check and Open
		if !klinePauser.Value() && !timeoutPauser.Value() {
			t.status = PROCESSOPEN
			t.closeRatio, t.openStableCoinPrice, t.openBookTickerASymbolPrice, t.openBookTickerBSymbolPrice,
				t.openID = t.open()
		}
	} else {
		if t.status == FILLED ||
			(t.status == UNFILLED && !t.fok) {
			// Close
			ctx, cancel := context.WithTimeout(context.Background(), t.closeTimeOut)
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
				if profit.IsPositive() {
					t.gain = t.gain.Add(profit)
				} else {
					t.deficit = t.deficit.Add(profit)
				}

				t.profit = t.gain.Add(t.deficit)

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
	if t.onlyMode1 {
		t.status = RUNNING
		return decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, ""
	}

	t.status = PROCESSOPEN
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
	var (
		bookTickerASymbolBidPrice = t.bookTickerASymbolBidPrice
		bookTickerBSymbolAskPrice = t.bookTickerBSymbolAskPrice
		stableCoinSymbolBidPrice  = t.stableCoinSymbolBidPrice
	)

	ratioMode1, ok := t.calculateRatioMode1(bookTickerASymbolBidPrice, bookTickerBSymbolAskPrice, stableCoinSymbolBidPrice)
	if !ok {
		t.status = RUNNING
		return decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, ""
	}

	if ratioMode1.GreaterThanOrEqual(t.ratioMin) &&
		ratioMode1.LessThanOrEqual(t.ratioMax) {
		t.mode.Store(1)
		logrus.Info("[Open[]", t.ratioLog(ratioMode1, stableCoinSymbolBidPrice, bookTickerASymbolBidPrice, bookTickerBSymbolAskPrice))
		price := bookTickerBSymbolAskPrice.Add(t.fokStandard)
		if id, ok := t.tradeMode1(
			true,
			price,
			t.maxQty,
		); ok {
			t.status = FILLED
			return ratioMode1, t.stableCoinSymbolAskPrice, bookTickerASymbolBidPrice, bookTickerBSymbolAskPrice, id
		} else {
			t.status = UNFILLED
			return decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, ""
		}
	}

	t.status = RUNNING
	return decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, ""
}

// Mode2
func (t *Task) openMode2() (decimal.Decimal, decimal.Decimal, decimal.Decimal, decimal.Decimal, string) {
	var (
		bookTickerASymbolAskPrice = t.bookTickerASymbolAskPrice
		bookTickerBSymbolBidPrice = t.bookTickerBSymbolBidPrice
		stableCoinSymbolAskPrice  = t.stableCoinSymbolAskPrice
	)

	ratioMode2, ok := t.calculateRatioMode2(bookTickerASymbolAskPrice, bookTickerBSymbolBidPrice, stableCoinSymbolAskPrice)
	if !ok {
		t.status = RUNNING
		return decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, ""
	}

	if ratioMode2.GreaterThanOrEqual(t.ratioMin) &&
		ratioMode2.LessThanOrEqual(t.ratioMax) {
		t.mode.Store(2)
		logrus.Info("[Open[]", t.ratioLog(ratioMode2, stableCoinSymbolAskPrice, bookTickerASymbolAskPrice, bookTickerBSymbolBidPrice))
		price := bookTickerBSymbolBidPrice.Sub(t.fokStandard)
		if id, ok := t.tradeMode2(
			true,
			price,
			t.maxQty,
		); ok {

			t.status = FILLED
			return ratioMode2, t.stableCoinSymbolBidPrice, bookTickerASymbolAskPrice, bookTickerBSymbolBidPrice, id
		} else {
			t.status = UNFILLED
			return decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, ""
		}
	}

	t.status = RUNNING
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
				if t.openBookTickerBSymbolPrice.Sub(t.bookTickerBSymbolAskPrice).Cmp(decimal.NewFromFloat(1.0)) > 0 {
					if id, ok := t.tradeMode2(
						false,
						t.bookTickerBSymbolBidPrice,
						t.maxQty,
					); ok {

						logrus.Infof("Mode1 loss after open, force close. Open price: %s, ask: %s, amount of loss: %s",
							t.openBookTickerBSymbolPrice.String(),
							t.bookTickerBSymbolAskPrice.String(),
							t.openBookTickerBSymbolPrice.Sub(t.bookTickerBSymbolAskPrice).String(),
						)
						t.expectProfitLog(t.bookTickerBSymbolAskPrice)
						return id
					}
				}

				// Do mode 2
				var (
					bookTickerASymbolAskPrice = t.bookTickerASymbolAskPrice
					bookTickerBSymbolBidPrice = t.bookTickerBSymbolBidPrice
					openStableCoinPrice       = t.openStableCoinPrice
				)
				ratio := decimal.NewFromFloat(-0.0001).Sub(t.closeRatio).Mul(t.ratioProfit)
				ratioMode2, ok := t.calculateRatioMode2(bookTickerASymbolAskPrice, bookTickerBSymbolBidPrice, openStableCoinPrice)
				if !ok {
					continue
				}

				if ratioMode2.GreaterThanOrEqual(ratio) {
					logrus.Info("[Close[]", t.ratioLog(ratioMode2, openStableCoinPrice, bookTickerASymbolAskPrice, bookTickerBSymbolBidPrice))
					t.expectProfitLog(t.bookTickerBSymbolBidPrice)

					// Trade
					if id, ok := t.tradeMode2(
						false,
						bookTickerBSymbolBidPrice,
						t.maxQty,
					); ok {
						return id
					}
				}
			case 2:
				if t.openBookTickerBSymbolPrice.Sub(t.bookTickerBSymbolBidPrice).Cmp(decimal.NewFromFloat(-1.0)) < 0 {
					if id, ok := t.tradeMode1(
						false,
						t.bookTickerBSymbolAskPrice,
						t.maxQty,
					); ok {
						logrus.Infof("Mode2 loss after open, force close. Open price: %s, bid: %s, amount of loss: %s",
							t.openBookTickerBSymbolPrice.String(),
							t.bookTickerBSymbolBidPrice.String(),
							t.openBookTickerBSymbolPrice.Sub(t.bookTickerBSymbolBidPrice).String(),
						)
						t.expectProfitLog(t.bookTickerBSymbolBidPrice)
						return id
					}
				}

				// Do mode 1
				var (
					bookTickerASymbolBidPrice = t.bookTickerASymbolBidPrice
					bookTickerBSymbolAskPrice = t.bookTickerBSymbolAskPrice
					openStableCoinPrice       = t.openStableCoinPrice
				)

				ratio := decimal.NewFromFloat(-0.0001).Sub(t.closeRatio).Mul(t.ratioProfit)
				ratioMode1, ok := t.calculateRatioMode1(bookTickerASymbolBidPrice, bookTickerBSymbolAskPrice, openStableCoinPrice)
				if !ok {
					continue
				}

				if ratioMode1.GreaterThanOrEqual(ratio) {
					logrus.Info("[Close[]:", t.ratioLog(ratioMode1, t.openStableCoinPrice, bookTickerASymbolBidPrice, bookTickerBSymbolAskPrice))
					t.expectProfitLog(t.bookTickerBSymbolAskPrice)

					// Trade
					if id, ok := t.tradeMode1(
						false,
						bookTickerBSymbolAskPrice,
						t.maxQty,
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
		var (
			bookTickerASymbolAskPrice = t.bookTickerASymbolAskPrice
			bookTickerBSymbolBidPrice = t.bookTickerBSymbolBidPrice
			openStableCoinPrice       = t.openStableCoinPrice
		)

		ratio, _ := t.calculateRatioMode2(bookTickerASymbolAskPrice, bookTickerBSymbolBidPrice, openStableCoinPrice)
		logrus.Info("[Force close[]",
			t.ratioLog(
				ratio,
				openStableCoinPrice,
				bookTickerASymbolAskPrice,
				bookTickerBSymbolBidPrice,
			),
		)
		t.expectProfitLog(bookTickerBSymbolBidPrice)

		if id, ok := t.tradeMode2(
			false,
			bookTickerBSymbolBidPrice,
			t.maxQty,
		); ok {
			return id
		}
	case 2:
		var (
			bookTickerBSymbolBidPrice = t.bookTickerBSymbolBidPrice
			bookTickerBSymbolAskPrice = t.bookTickerBSymbolAskPrice
			openStableCoinPrice       = t.openStableCoinPrice
		)
		ratio, _ := t.calculateRatioMode1(bookTickerBSymbolBidPrice, bookTickerBSymbolAskPrice, openStableCoinPrice)
		logrus.Info("[Force close[]",
			t.ratioLog(
				ratio,
				openStableCoinPrice,
				bookTickerBSymbolBidPrice,
				bookTickerBSymbolAskPrice,
			),
		)
		t.expectProfitLog(bookTickerBSymbolAskPrice)

		if id, ok := t.tradeMode1(
			false,
			bookTickerBSymbolAskPrice,
			t.maxQty,
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
	if isOpen && t.fok {
		if t.future {
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

	if t.future {
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
	if isOpen && t.fok {
		if t.future {
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

	if t.future {
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
		if strings.Contains(err.Error(), "code=-2010") {
			t.UpdateBalance()
		}
		logrus.Errorf("Spot FOK Order --- Error: %v, %s", res, err)
		time.Sleep(time.Millisecond * 50)
		return "", false
	}

	switch res.Status {
	case binancesdk.OrderStatusTypeExpired:
		logrus.Error("Spot FOK Order --- Failed，Expired")
		time.Sleep(time.Millisecond * 50)
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
		if strings.Contains(err.Error(), "code=-2010") {
			t.UpdateBalance()
		}
		logrus.Errorf("Future FOK Order --- Error: %v, %s", res, err.Error())
		time.Sleep(time.Millisecond * 50)
		return "", false
	}

	switch res.Status {
	case futures.OrderStatusTypeExpired:
		logrus.Error("Future FOK Order --- Failed，Expired")
		time.Sleep(time.Millisecond * 50)
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

type TaskControl interface {
	GetInfo() string
	Stop()
	Start()

	MaxQty()
	CycleNumber()
	WaitDuration()
	CloseTimeOut()
	FOK()
	FOKStandard()
	Future()
	OnlyMode1()
	RatioMin()
	RatioMax()
	RatioProfit()
	UpdateBalance()
}

func (t *Task) GetInfo() string {
	return fmt.Sprintf(
		"Status:%s|Progress:%d/%d|Gain:%s|Profit:%s|Deficit:%s|Mode1:%s|Mode2:%s",
		t.status.String(),
		t.completedCnt,
		t.cycleNumber,
		t.gain.String(),
		t.profit.String(),
		t.deficit.String(),
		t.mode1Ratio.String(),
		t.mode2Ratio.String(),
	)
}

func (t *Task) Stop() {
	go func() {
		for {
			if t.status == RUNNING {
				t.status = STOP
				return
			}
		}
	}()
}

func (t *Task) Start() {
	if t.completedCnt >= t.cycleNumber {
		t.completedCnt = 0
	}
	t.status = RUNNING
}

func (t *Task) MaxQty() {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.maxQty = viper.GetString("MaxQty")
}

func (t *Task) CycleNumber() {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.cycleNumber = viper.GetInt("CycleNumber")
}

func (t *Task) WaitDuration() {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.waitDuration = viper.GetDuration("WaitDuration")
}

func (t *Task) CloseTimeOut() {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.closeTimeOut = viper.GetDuration("CloseTimeOut")
}

func (t *Task) FOK() {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.fok = viper.GetBool("FOK")
}

func (t *Task) FOKStandard() {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.fokStandard = viper.Get("FOKStandard").(decimal.Decimal)
}

func (t *Task) Future() {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.future = viper.GetBool("Future")
}

func (t *Task) OnlyMode1() {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.onlyMode1 = viper.GetBool("OnlyMode1")
}

func (t *Task) RatioMin() {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.ratioMin = viper.Get("RatioMin").(decimal.Decimal)
}

func (t *Task) RatioMax() {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.ratioMax = viper.Get("RatioMax").(decimal.Decimal)
}

func (t *Task) RatioProfit() {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.ratioProfit = viper.Get("RatioProfit").(decimal.Decimal)
}

func (t *Task) UpdateBalance() {
	qty := t.Balance.BalanceUpdate()
	if qty != "" {
		viper.Set("MaxQty", qty)
		t.maxQty = viper.GetString("MaxQty")
	}
}
