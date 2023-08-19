package account

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/isther/arbitrage-htc/utils"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/vicanso/go-charts/v2"
)

type CntInputer interface {
	AddOrder(profit decimal.Decimal, t time.Time)
}

type CntOutputer interface {
	Chart() string
}

type Counter struct {
	totalProfit decimal.Decimal

	times        []time.Time
	profitValues []float64

	l sync.RWMutex
}

func NewCounter() *Counter {
	return &Counter{
		totalProfit:  decimal.Zero,
		times:        []time.Time{time.Now()},
		profitValues: []float64{0.0},
		l:            sync.RWMutex{},
	}
}

func (c *Counter) AddOrder(profit decimal.Decimal, t time.Time) {
	c.l.Lock()
	defer c.l.Unlock()

	var (
		qty          = utils.StringToDecimal(viper.GetString("MaxQty"))
		truelyProfit = profit.Mul(qty)
	)

	c.totalProfit = c.totalProfit.Add(truelyProfit)
	p, _ := c.totalProfit.Add(truelyProfit).Float64()

	c.times = append(c.times, t)
	c.profitValues = append(c.profitValues, p)
}

func (c *Counter) Chart() string {
	c.l.RLock()
	defer c.l.RUnlock()

	var (
		imgFilePath = filepath.Join("./imgs", fmt.Sprintf("counter%d.png", time.Now().UTC().UnixMilli()))
		xAxisValue  = []string{}
		firstAxis   = 0
	)

	for i, t := range c.times {
		if firstAxis == 0 {
			firstAxis = i
		}
		xAxisValue = append(xAxisValue, t.Add(8*time.Hour).Format("15:04"))
	}

	p, err := charts.LineRender(
		[][]float64{c.profitValues},
		charts.FontFamilyOptionFunc("noto"),
		charts.TitleTextOptionFunc("Profit"),
		charts.LegendLabelsOptionFunc([]string{"Profit"}, "128"),
		charts.XAxisDataOptionFunc(xAxisValue),
		func(opt *charts.ChartOption) {
			opt.Theme = "dark"
			opt.LineStrokeWidth = 1
			opt.FillArea = true
			opt.XAxis.FirstAxis = firstAxis
			opt.XAxis.SplitNumber = 1
			opt.Legend.Padding = charts.Box{
				Top:    5,
				Bottom: 10,
			}
			opt.SymbolShow = charts.FalseFlag()
			opt.ValueFormatter = func(f float64) string { return fmt.Sprintf("%.8fU", f) }
		},
	)
	if err != nil {
		logrus.Info(err)
	}

	f, _ := os.Create(imgFilePath)
	defer f.Close()

	buf, err := p.Bytes()
	if err != nil {
		logrus.Info(err)
	}

	f.Write(buf)

	return imgFilePath
}
