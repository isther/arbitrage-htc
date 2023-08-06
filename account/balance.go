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
	BalanceInfo() map[string]string
}

func (b *BALANCE) BalanceInfo() map[string]string {
	b.L.Lock()
	defer b.L.Unlock()

	return b.balances
}

type BALANCE struct {
	isFuture      *bool
	useBNB        *bool
	bnbMinQty     *float64
	autoBuyBNB    *bool
	autoBuyBNBQty *float64 // U

	balances        map[string]string
	updateBalanceCh chan struct{}

	balanceViews []BalanceView

	L sync.RWMutex
}

func NewBALANCE(
	isFuture *bool,
	useBNB *bool,
	bnbMinQty *float64,
	autoBuyBNB *bool,
	autoBuyBNBQty *float64, // U
) *BALANCE {
	return &BALANCE{
		isFuture:      isFuture,
		useBNB:        useBNB,
		bnbMinQty:     bnbMinQty,
		autoBuyBNB:    autoBuyBNB,
		autoBuyBNBQty: autoBuyBNBQty,

		balances:        make(map[string]string),
		updateBalanceCh: make(chan struct{}),
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

	if *b.isFuture {
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
		panic("获取币安现货余额数量失败: " + err.Error())
	}

	for _, v := range res.Balances {
		if *b.useBNB && v.Asset == "BNB" {
			free := utils.StringToDecimal(v.Free)
			if free.LessThan(decimal.NewFromFloat(*b.bnbMinQty)) {
				if !next && *b.autoBuyBNB {
					b.tradeWithQuoteQty(getSymbol([2]string{assets[0], "BNB"}), binancesdk.SideTypeBuy, decimal.NewFromFloat(*b.autoBuyBNBQty))
					logrus.Infof("BNB不足---尝试使用%s购买", assets[0])
					b.updateBinanceSpotBalance(true, assets...)
					return
				}

				logrus.Warn("BNB不足---程序停止")
				time.Sleep(time.Second * 4)
				panic("币安现货bnb数量不足")
			}
		}

		for _, asset := range assets {
			if v.Asset == asset {
				b.balances[asset] = v.Free
			}
		}

	}
}

func (b *BALANCE) updateBinanceFuturesBalance(next bool, assets ...string) {
	res, err := utils.NewBinanceFuturesClient().NewGetAccountService().Do(context.Background())
	if err != nil {
		panic("获取币安合约钱包余额失败: " + err.Error())
	}

	for _, v := range res.Assets {
		if *b.autoBuyBNB && v.Asset == "BNB" {
			free := utils.StringToDecimal(v.WalletBalance)
			if free.LessThan(decimal.NewFromFloat(*b.bnbMinQty)) {
				if !next && *b.autoBuyBNB {
					// buy
					b.tradeWithQuoteQty(getSymbol([2]string{assets[0], "BNB"}), binancesdk.SideTypeBuy, decimal.NewFromFloat(*b.autoBuyBNBQty))
					logrus.Infof("BNB不足---尝试使用%v购买", assets[0])

					// BUG: transfer to futures
					utils.NewBinanceClient().NewFuturesTransferService().Asset("BNB").Amount("").Type(binancesdk.FuturesTransferTypeToFutures).Do(context.Background())
					b.updateBinanceFuturesBalance(true, assets...)
					return
				}

				logrus.Warn("BNB不足---程序停止")
				time.Sleep(time.Second * 4)
				panic("币安合约bnb数量不足")
			}
		}

		for _, asset := range assets {
			if v.Asset == asset {
				b.balances[asset] = v.WalletBalance
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
