package account

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/isther/arbitrage-htc/utils"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	binancesdk "github.com/adshao/go-binance/v2"
)

type Balance interface {
	Run()
	BalanceUpdate() string
	AddBalanceView(BalanceView)
	BalanceInfo
}

type BalanceView interface {
	View(map[string]string)
}

type BalanceInfo interface {
	GetBalanceInfo() map[string]string
}

func (b *BALANCE) GetBalanceInfo() map[string]string {
	b.L.Lock()
	defer b.L.Unlock()

	return b.balances
}

type BALANCE struct {
	assets          []string
	balances        map[string]string
	updateBalanceCh chan struct{}

	balanceViews []BalanceView

	start bool

	L sync.RWMutex
}

func NewBALANCE() *BALANCE {
	return &BALANCE{
		assets:          []string{"USDT", "BTC", "BNB"},
		balances:        make(map[string]string),
		updateBalanceCh: make(chan struct{}),
		balanceViews:    make([]BalanceView, 0),
		start:           true,
		L:               sync.RWMutex{},
	}
}

func (b *BALANCE) Run() {
	b.AddBalanceView(&defaultBalanceView{})
}

func (b *BALANCE) AddBalanceView(view BalanceView) {
	b.L.Lock()
	defer b.L.Unlock()

	b.balanceViews = append(b.balanceViews, view)
}

func (b *BALANCE) BalanceUpdate() string {
	b.L.RLock()
	defer b.L.RUnlock()

	var (
		qty string
	)

	if viper.GetBool("Future") {
		qty = b.updateBinanceFuturesBalance(b.assets...)
	} else {
		qty = b.updateBinanceSpotBalance(b.assets...)
	}

	for _, view := range b.balanceViews {
		view.View(b.balances)
	}

	return qty
}

func (b *BALANCE) updateBinanceSpotBalance(assets ...string) string {
	res, err := utils.NewBinanceClient().NewGetAccountService().Do(context.Background())
	if err != nil {
		logrus.Error("Get binance spot balances failed: " + err.Error())
		return ""
	}

	// update balances
	for _, v := range res.Balances {
		for _, asset := range assets {
			if v.Asset == asset {
				b.balances[asset] = v.Free
			}
		}
	}

	return b.check(b.assets...)
}

// check
func (b *BALANCE) check(assets ...string) string {
	// get btc price
	priceRes, err := utils.NewBinanceClient().NewListPricesService().Symbol("BTCUSDT").Do(context.Background())
	if err != nil {
		logrus.Error(err)
		return ""
	}

	var price decimal.Decimal
	for _, v := range priceRes {
		if v.Symbol == "BTCUSDT" {
			price = utils.StringToDecimal(v.Price)
		}
	}

	qty, ok := b.balance(price)
	if ok {
		if utils.StringToDecimal(b.balances["BNB"]).LessThan(viper.Get("BNBMinQty").(decimal.Decimal)) {
			if viper.GetBool("AutoBuyBNB") {
				b.tradeWithQuoteQty(getSymbol([2]string{assets[0], "BNB"}), binancesdk.SideTypeBuy, viper.Get("AutoBuyBNBQty").(decimal.Decimal))
				logrus.Infof("BNB not sufficient --- try buying with %s", assets[0])
				return qty
			} else {
				logrus.Error("BNB not sufficient --- Program stop")
				time.Sleep(1 * time.Second)
				panic("BNB not sufficient --- Program stop")
			}
		}
	}
	return qty
}

func (b *BALANCE) balance(price decimal.Decimal) (string, bool) {
	var (
		usdt          = utils.StringToDecimal(b.balances["USDT"])
		btc           = utils.StringToDecimal(b.balances["BTC"])
		maxQty        = utils.StringToDecimal(viper.GetString("MaxQty"))
		autobuyBNBQty = viper.Get("AutoBuyBNBQty").(decimal.Decimal)
		adjustSize    = decimal.NewFromFloat(0.0001)
	)

	maxQty = decimal.Min(maxQty, btc)

	for usdt.Sub(maxQty.Mul(price)).LessThan(autobuyBNBQty) {
		maxQty = maxQty.Sub(adjustSize)
	}

	if maxQty.Mul(price).LessThan(decimal.NewFromFloat(12.0)) {
		return "", false
	} else {
		return maxQty.Truncate(4).String(), true
	}
}

func (b *BALANCE) updateBinanceFuturesBalance(assets ...string) string { //TODO:
	return ""
}

func (b *BALANCE) tradeWithQuoteQty(symbol string, side binancesdk.SideType, quoteQty decimal.Decimal) {
	res, err := utils.NewBinanceClient().
		NewCreateOrderService().
		Symbol(symbol).
		Side(side).
		Type(binancesdk.OrderTypeMarket).
		QuoteOrderQty(quoteQty.String()).
		Do(context.Background())
	if err != nil {
		logrus.Errorf("CreateOrderService error: %v", err.Error())
	}
	_ = res
}

func getSymbol(assets [2]string) string {
	if strings.Compare(assets[0], assets[1]) > 0 {
		return assets[1] + assets[0]
	}
	return assets[0] + assets[1]
}

type defaultBalanceView struct{}

func (d *defaultBalanceView) View(balances map[string]string) {
	var (
		balance = ""
	)
	// list = append(list, fmt.Sprintf(""))
	for k, v := range balances {
		balance += fmt.Sprintf("%s:%s ", k, v)
	}

	logrus.Info("Balance: " + balance)
}
