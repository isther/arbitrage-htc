package account

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	binancesdk "github.com/adshao/go-binance/v2"
	"github.com/isther/arbitrage-htc/utils"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Balance interface {
	Run()
	BalanceUpdate()
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
	balances        map[string]string
	updateBalanceCh chan struct{}

	canBuy bool

	balanceViews []BalanceView

	L sync.RWMutex
}

func NewBALANCE() *BALANCE {
	return &BALANCE{
		balances:        make(map[string]string),
		updateBalanceCh: make(chan struct{}),
		canBuy:          true,
		balanceViews:    make([]BalanceView, 0),
		L:               sync.RWMutex{},
	}
}

func (b *BALANCE) Run() {
	b.AddBalanceView(&defaultBalanceView{})

	b.updateBinanceBalance()

	go func() {
		for {
			<-b.updateBalanceCh
			b.updateBinanceBalance()
		}
	}()
}

func (b *BALANCE) AddBalanceView(view BalanceView) {
	b.L.Lock()
	defer b.L.Unlock()

	b.balanceViews = append(b.balanceViews, view)
}

func (b *BALANCE) BalanceUpdate() {
	b.updateBalanceCh <- struct{}{}
}

func (b *BALANCE) updateBinanceBalance() {
	b.L.Lock()
	defer b.L.Unlock()

	if viper.GetBool("Future") {
		b.updateBinanceFuturesBalance(false, "USDT", "BTC", "BNB")
	} else {
		b.updateBinanceSpotBalance(false, "USDT", "BTC", "BNB")
	}

	for _, view := range b.balanceViews {
		view.View(b.balances)
	}
}

func (b *BALANCE) updateBinanceSpotBalance(next bool, assets ...string) {
	res, err := utils.NewBinanceClient().NewGetAccountService().Do(context.Background())
	if err != nil {
		logrus.Error("Get binance spot balances failed: " + err.Error())
		return
	}

	for _, v := range res.Balances {
		if viper.GetBool("UseBNB") && v.Asset == "BNB" {
			free := utils.StringToDecimal(v.Free)
			if free.LessThan(viper.Get("BNBMinQty").(decimal.Decimal)) {
				if !next &&
					viper.GetBool("AutoBuyBNB") &&
					b.canBuy {
					b.tradeWithQuoteQty(getSymbol([2]string{assets[0], "BNB"}), binancesdk.SideTypeBuy, viper.Get("AutoBuyBNBQty").(decimal.Decimal))
					logrus.Infof("BNB not sufficient --- try buying with %s", assets[0])
					b.updateBinanceSpotBalance(true, assets...)
					return
				}
				logrus.Error("BNB and USDT not sufficient --- Program stop")
				time.Sleep(time.Second * 1)
				panic("BNB and USDT not sufficient --- Program stop")
			}
		}

		for _, asset := range assets {
			if v.Asset == asset {
				b.balances[asset] = v.Free
			}
		}
	}

	var (
		usdt = utils.StringToDecimal(b.balances["USDT"])
		btc  = utils.StringToDecimal(b.balances["BTC"])
	)
	priceRes, err := utils.NewBinanceClient().NewListPricesService().Symbol("BTCUSDT").Do(context.Background())
	if err != nil {
		logrus.Error(err)
		return
	}
	for _, v := range priceRes {
		if v.Symbol == "BTCUSDT" {
			price := utils.StringToDecimal(v.Price)
			if usdt.Sub(btc.Mul(price)).Abs().LessThan(viper.Get("AutoBuyBNBQty").(decimal.Decimal)) {
				b.canBuy = false
			}
		}
	}
}

func (b *BALANCE) updateBinanceFuturesBalance(next bool, assets ...string) {
	res, err := utils.NewBinanceFuturesClient().NewGetAccountService().Do(context.Background())
	if err != nil {
		logrus.Error("Get binance future balances failed: " + err.Error())
		return
	}

	for _, v := range res.Assets {
		if viper.GetBool("AutoBuyBNB") && v.Asset == "BNB" {
			free := utils.StringToDecimal(v.WalletBalance)
			if free.LessThan(viper.Get("BNBMinQty").(decimal.Decimal)) {
				if !next &&
					viper.GetBool("AutoBuyBNB") &&
					b.canBuy {
					// buy
					b.tradeWithQuoteQty(getSymbol([2]string{assets[0], "BNB"}), binancesdk.SideTypeBuy, viper.Get("AutoBuyBNBQty").(decimal.Decimal))
					logrus.Infof("BNB not sufficient --- try buying with %s", assets[0])

					// BUG: transfer to futures
					utils.NewBinanceClient().NewFuturesTransferService().Asset("BNB").Amount("").Type(binancesdk.FuturesTransferTypeToFutures).Do(context.Background())
					b.updateBinanceFuturesBalance(true, assets...)
					return
				}
				logrus.Error("BNB and USDT not sufficient --- Program stop")
				time.Sleep(time.Second * 1)
				panic("BNB and USDT not sufficient --- Program stop")
			}
		}

		for _, asset := range assets {
			if v.Asset == asset {
				b.balances[asset] = v.WalletBalance
			}
		}
	}

	var (
		usdt = utils.StringToDecimal(b.balances["USDT"])
		btc  = utils.StringToDecimal(b.balances["BTC"])
	)
	priceRes, err := utils.NewBinanceClient().NewListPricesService().Symbol("BTCUSDT").Do(context.Background())
	if err != nil {
		logrus.Error(err)
		return
	}
	for _, v := range priceRes {
		if v.Symbol == "BTCUSDT" {
			price := utils.StringToDecimal(v.Price)
			if usdt.Sub(btc.Mul(price)).Abs().LessThan(viper.Get("AutoBuyBNBQty").(decimal.Decimal)) {
				b.canBuy = false
			}
		}
	}
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
