package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	binancesdk "github.com/adshao/go-binance/v2"
	"github.com/isther/arbitrage-htc/account"
	"github.com/isther/arbitrage-htc/core"
	"github.com/isther/arbitrage-htc/telbot"
	"github.com/isther/arbitrage-htc/utils"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var (
	configPath = ".config.json"
)

func init() {
	// Binance
	viper.SetDefault("Telegram.Token", os.Getenv("TG_TOKEN"))
	viper.SetDefault("Binance.Api", os.Getenv("BINANCE_API"))
	viper.SetDefault("Binance.Secret", os.Getenv("BINANCE_SECRET"))

	// Symbols
	viper.SetDefault("BookTickerASymbol", "BTCTUSD")
	viper.SetDefault("StableCoinSymbol", "TUSDUSDT")
	viper.SetDefault("BookTickerBSymbol", "BTCUSDT")

	// Mode
	viper.SetDefault("FOK", true)
	viper.SetDefault("Future", false)
	viper.SetDefault("OnlyMode1", false)

	// Fee
	viper.SetDefault("UseBNB", true)
	viper.SetDefault("BNBMinQty", decimal.NewFromFloat(0.002))
	viper.SetDefault("AutoBuyBNB", true)
	viper.SetDefault("AutoBuyBNBQty", decimal.NewFromFloat(5.0))

	// Parmams
	viper.SetDefault("CycleNumber", 100)
	viper.SetDefault("RatioMin", decimal.NewFromFloat(1.0))
	viper.SetDefault("RatioMax", decimal.NewFromFloat(10.0))
	viper.SetDefault("RatioProfit", decimal.NewFromFloat(0.5))
	viper.SetDefault("CloseTimeOut", 60000*time.Millisecond)
	viper.SetDefault("PauseMinKlineRatio", decimal.NewFromFloat(1.0))
	viper.SetDefault("PauseMaxKlineRatio", decimal.NewFromFloat(100.0))
	viper.SetDefault("PauseClientTimeOutLimit", 100)
	viper.SetDefault("WaitDuration", 3000*time.Millisecond)
	viper.SetDefault("FOKStandard", decimal.NewFromFloat(1.0))
	viper.SetDefault("MaxQty", "0.0004")
	viper.SetDefault("AutoAdjustQty", true)

	viper.SetConfigFile(configPath)
	viper.ReadInConfig()

	updateValue := func(key string) {
		v := viper.GetFloat64(key)
		if v != 0 {
			viper.Set(key, decimal.NewFromFloat(v))
		}
	}
	updateValue("BNBMinQty")
	updateValue("AutoBuyBNBQty")
	updateValue("RatioMin")
	updateValue("RatioMax")
	updateValue("RatioProfit")
	updateValue("PauseMinKlineRatio")
	updateValue("PauseMaxKlineRatio")
	updateValue("FOKStandard")

	utils.InitBinanceClientApi()

	file, err := os.OpenFile(
		fmt.Sprintf(
			"./logs/logs-%v-%v-%v-%v:%v:%v.log",
			time.Now().UTC().Year(),
			time.Now().UTC().Month(),
			time.Now().UTC().Day(),
			time.Now().UTC().Hour(),
			time.Now().UTC().Minute(),
			time.Now().UTC().Second(),
		),
		os.O_WRONLY|os.O_CREATE,
		0755,
	)
	if err != nil {
		log.Fatalf("create file log.txt failed: %v", err)
	}
	logrus.SetOutput(io.MultiWriter(os.Stdout, file))

	// binance websocket keepalive
	binancesdk.WebsocketKeepalive = true
}

func main() {
	var (
		account = account.NewAccount()
		task    = core.NewTask(
			viper.GetString("Binance.Api"),
			viper.GetString("Binance.Secret"),
			viper.GetString("BookTickerASymbol"),
			viper.GetString("StableCoinSymbol"),
			viper.GetString("BookTickerBSymbol"),
			account.ExchangeInfo,
			account.Balance,
			account.OrderList,
		)
		arbitrageManager = core.NewArbitrageManager(
			viper.GetString("BookTickerASymbol"),
			viper.GetString("StableCoinSymbol"),
			viper.GetString("BookTickerBSymbol"),
		)
	)

	go account.Run()
	go task.Run()
	go arbitrageManager.Run()
	arbitrageManager.AddUpdateEvent(task)
	telBot := telbot.NewTelBot(viper.GetString("Telegram.Token"), account.Balance, task)
	logrus.AddHook(telBot)
	go telBot.Run()

	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	for s := range c {
		switch s {
		case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
			viper.SetDefault("Telegram.Token", nil)
			viper.SetDefault("Binance.Api", nil)
			viper.SetDefault("Binance.Secret", nil)
			viper.WriteConfig()

			logrus.Info("Exit: ", s)
			os.Exit(0)
		}
	}
}
