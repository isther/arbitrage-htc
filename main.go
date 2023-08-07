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
	"github.com/isther/arbitrage-htc/config"
	"github.com/isther/arbitrage-htc/core"
	"github.com/isther/arbitrage-htc/dingding"
	"github.com/isther/arbitrage-htc/telbot"
	"github.com/isther/arbitrage-htc/tui"
	"github.com/sirupsen/logrus"
)

func init() {
	config.Config.Load("config.yaml")

	logrus.SetReportCaller(false)
	logrus.SetFormatter(&config.LogFormatter{})
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
	if config.Config.UI {
		logrus.SetOutput(io.MultiWriter(file))
	} else {
		logrus.SetOutput(io.MultiWriter(os.Stdout, file))
	}

	// Add dingding bot hook
	if config.Config.DingDingLogConfig.AccessToken != "" && config.Config.DingDingLogConfig.Secrect != "" && config.Config.DingDingErrConfig.AccessToken != "" && config.Config.DingDingErrConfig.Secrect != "" {
		logrus.AddHook(dingding.NewDingDingBotHook(
			config.Config.DingDingLogConfig.AccessToken, config.Config.DingDingLogConfig.Secrect,
			config.Config.DingDingErrConfig.AccessToken, config.Config.DingDingErrConfig.Secrect,
			10000,
			3000, // ms
		))
	}

	// binance websocket keepalive
	binancesdk.WebsocketKeepalive = true
}

func main() {
	account := account.NewAccount(
		&config.Config.Mode.IsFuture,
		&config.Config.Fee.UseBNB,
		&config.Config.Fee.BNBMinQty,
		&config.Config.Fee.AutoBuyBNB,
		&config.Config.Fee.AutoBuyBNBQty,
	)
	go account.Run()

	task := core.NewTask(
		config.Config.Api,
		config.Config.Secret,
		&config.Config.IsFOK,
		&config.Config.IsFuture,
		&config.Config.OnlyMode1,
		&config.Config.MaxQty,
		&config.Config.CycleNumber,
		&config.Config.WaitDuration,
		&config.Config.CloseTimeOut,
		config.Config.RatioProfit,
		config.Config.RatioMin,
		config.Config.RatioMax,
		config.Config.BookTickerASymbol,
		config.Config.StableCoinSymbol,
		config.Config.BookTickerBSymbol,
		account.ExchangeInfo,
		account.Balance,
		account.OrderList,
	)
	// go task.Run()

	arbitrageManager := core.NewArbitrageManager(
		config.Config.Symbols.BookTickerASymbol,
		config.Config.Symbols.StableCoinSymbol,
		config.Config.Symbols.BookTickerBSymbol,
		config.Config.Params.PauseMaxKlineRatio,
		config.Config.Params.PauseMinKlineRatio,
		config.Config.Params.PauseClientTimeOutLimit,
	)
	arbitrageManager.AddUpdateEvent(task)
	go arbitrageManager.Run()

	if config.Config.Telegram.Token != "" {
		telBot := telbot.NewTelBot(config.Config.Telegram.Token, account.Balance, task)
		logrus.AddHook(telBot)
		go telBot.Run()
	}

	// Start
	if config.Config.Mode.UI {
		ui := tui.NewTui(
			[]string{config.Config.BookTickerASymbol, config.Config.StableCoinSymbol, config.Config.BookTickerBSymbol},
			func() {},
			[]core.TaskInfo{task},
		)
		{
			logrus.AddHook(ui.Logger)
			arbitrageManager.AddUpdateEvent(ui)
			account.Balance.AddBalanceView(ui)
		}
		ui.Run()
	}

	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	for s := range c {
		switch s {
		case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
			config.Config.Save("config.yaml")
			logrus.Info("Exit", s)
			os.Exit(0)
		}
	}
}
