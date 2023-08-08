package account

import (
	"context"
	"strings"
	"sync"
	"time"

	binancesdk "github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/futures"
	"github.com/isther/arbitrage-htc/utils"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type ExchangeInfo interface {
	Run()
	CorrectionPrice(symbol string, price decimal.Decimal) decimal.Decimal
	CorrectionQty(symbol string, qty decimal.Decimal) decimal.Decimal
}

type exchangeInfo struct {
	spotSymbols    map[string]binancesdk.Symbol
	futuresSymbols map[string]futures.Symbol
	lock           sync.RWMutex
}

func NewexchangeInfoer() *exchangeInfo {
	return &exchangeInfo{
		spotSymbols:    make(map[string]binancesdk.Symbol),
		futuresSymbols: make(map[string]futures.Symbol),
		lock:           sync.RWMutex{},
	}
}

func (e *exchangeInfo) Run() {
	// TODO:Add updateFuturesExchangeInfo
	if viper.GetBool("Future") {
		return
	}

	err := e.updateSpotExchangeInfo()
	if err != nil {
		logrus.Errorf("updateExchangeInfo error: %v", err.Error())
	}

	go func() {
		for {
			if err := e.updateSpotExchangeInfo(); err == nil {
				time.Sleep(30 * time.Minute)
			} else {
				logrus.Errorf("updateExchangeInfo error: %v", err.Error())
			}
		}
	}()
}

// get ExchangeInfo
func (e *exchangeInfo) updateSpotExchangeInfo() error {
	exchangeInfo, err := utils.NewBinanceClient().NewExchangeInfoService().Do(context.Background())
	if err != nil {
		return err
	}
	for i := range exchangeInfo.Symbols {
		e.spotSymbols[exchangeInfo.Symbols[i].Symbol] = exchangeInfo.Symbols[i]
	}

	return nil
}

// get binance ExchangeInfo
func (e *exchangeInfo) updateFuturesExchangeInfo() error {
	exchangeInfo, err := utils.NewBinanceFuturesClient().NewExchangeInfoService().Do(context.Background())
	if err != nil {
		return err
	}
	for i := range exchangeInfo.Symbols {
		e.futuresSymbols[exchangeInfo.Symbols[i].Symbol] = exchangeInfo.Symbols[i]
	}

	return nil
}

func (e *exchangeInfo) CorrectionPrice(symbol string, price decimal.Decimal) decimal.Decimal {
	if viper.GetBool("Future") {
		symbol_, ok := e.futuresSymbols[symbol]
		if ok {
			if symbol_.Symbol == symbol {
				priceFilter := symbol_.PriceFilter()
				return correction(price, priceFilter.TickSize)
			}
		}
	} else {
		symbol_, ok := e.spotSymbols[symbol]
		if ok {
			if symbol_.Symbol == symbol {
				priceFilter := symbol_.PriceFilter()
				return correction(price, priceFilter.TickSize)
			}
		}
	}

	panic("not found symbol")
}

func (e *exchangeInfo) CorrectionQty(symbol string, qty decimal.Decimal) decimal.Decimal {
	if viper.GetBool("Future") {
		symbol_, ok := e.futuresSymbols[symbol]
		if ok {
			if symbol_.Symbol == symbol {
				lotSize := symbol_.LotSizeFilter()
				return correction(qty, lotSize.StepSize)
			}
		}
	} else {
		symbol_, ok := e.spotSymbols[symbol]
		if ok {
			if symbol_.Symbol == symbol {
				lotSize := symbol_.LotSizeFilter()
				return correction(qty, lotSize.StepSize)
			}
		}
	}

	panic("not found symbol")
}

func correction(val decimal.Decimal, size string) decimal.Decimal {
	var (
		oneIdx    = strings.Index(size, "1")
		pointIdx  = strings.Index(size, ".")
		precision int
	)
	if oneIdx < pointIdx {
		precision = oneIdx - pointIdx + 1
	} else {
		precision = oneIdx - pointIdx
	}
	return val.Truncate(int32(precision)).Truncate(8)
}
