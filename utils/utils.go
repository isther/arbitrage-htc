package utils

import (
	binancesdk "github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/futures"
	"github.com/isther/arbitrage-htc/config"
	"github.com/shopspring/decimal"
)

func StringToDecimal(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func NewBinanceClient() *binancesdk.Client {
	return binancesdk.NewClient(config.Config.Api, config.Config.Secret)
}

func NewBinanceFuturesClient() *futures.Client {
	return futures.NewClient(config.Config.Api, config.Config.Secret)
}
