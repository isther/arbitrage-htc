package utils

import (
	"strings"

	binancesdk "github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/futures"
	"github.com/fogleman/gg"
	"github.com/shopspring/decimal"
	"github.com/spf13/viper"
)

func StringToDecimal(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func NewBinanceClient() *binancesdk.Client {
	return binancesdk.NewClient(viper.GetString("Binance.Api"), viper.GetString("Binance.Secret"))
}

func NewBinanceFuturesClient() *futures.Client {
	return futures.NewClient(viper.GetString("Binance.Api"), viper.GetString("Binance.Secret"))
}

func CreatePNG(content, filePath string) {
	var (
		maxLen int
		cnt             = strings.Count(content, "\n")
		lines  []string = make([]string, 0)
	)

	for _, line := range strings.Split(content, "\n") {
		lines = append(lines, line)

		l := len(line)
		if l > maxLen {
			maxLen = l
		}
	}

	var (
		points float64 = 20
		width          = 50 + maxLen*int(points)
		height         = 50 + cnt*int(points)
	)

	dc := gg.NewContext(width, height)
	dc.SetRGB(1, 1, 1)
	dc.Clear()
	dc.SetRGB(0, 0, 0)

	if err := dc.LoadFontFace("./ttf/BigBlueTerm437NerdFont-Regular.ttf", points); err != nil {
		panic(err)
	}

	for i, line := range lines {
		dc.DrawString(line, 50, 50+float64(i)*points)
	}
	dc.SavePNG(filePath)
}
