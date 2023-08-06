package tui

import (
	"fmt"
	"strconv"
	"time"

	"github.com/isther/arbitrage-htc/account"
	"github.com/isther/arbitrage-htc/config"
	"github.com/isther/arbitrage-htc/core"
	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
)

type Logger interface {
	Item() *tview.TextView
	logrus.Hook
}

type BookTicker interface {
	Item() *tview.Table
	core.BookTickerEventUpdater
}

type Balancer interface {
	Item() *tview.Table
	account.BalanceView
}

type TaskInfo interface {
	Item() *tview.Table
	Update()
}

type Tui struct {
	app           *tview.Application
	closeCallBack func()
	Logger
	BookTicker
	Balancer
	TaskInfo
}

func NewTui(
	symbols []string,
	closeCallBack func(),
	taskInfoView []core.TaskInfoView,
) *Tui {
	app := tview.NewApplication()
	return &Tui{
		app:           app,
		closeCallBack: closeCallBack,
		Logger:        NewTuiLogger(),
		BookTicker:    NewTuiBookTickerUpdater(symbols),
		Balancer:      NewTuiBalance(),
		TaskInfo:      NewTuiTaskInfo(taskInfoView),
	}
}

func (t *Tui) Run() {
	flex := tview.NewFlex().
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(t.BookTicker.Item(), 5, 0, false).
			AddItem(t.Balancer.Item(), 5, 0, false).
			AddItem(t.configForm(), 0, 1, false),
			50,
			1,
			false,
		).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(t.Logger.Item(), 0, 3, false).
			AddItem(t.TaskInfo.Item(), 0, 1, false),
			0,
			1,
			false,
		)

	go func() {
		var refreshInterval = 500 * time.Millisecond
		time.Sleep(refreshInterval)
		for {
			t.app.Draw()
			t.TaskInfo.Update()
			time.Sleep(refreshInterval)
		}
	}()

	if err := t.app.SetRoot(flex, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}

func (t *Tui) configForm() *tview.Form {
	form := tview.NewForm().
		//  Fee
		AddCheckbox("UseBNB", config.Config.UseBNB, func(checked bool) { config.Config.UseBNB = checked }).
		AddInputField("BNBMinQty", fmt.Sprintln(config.Config.BNBMinQty), 8, nil, func(text string) {
			num, err := strconv.ParseFloat(text, 64)
			if err != nil {
				// BUG:Add error output
				return
			}
			config.Config.BNBMinQty = num
		}).AddCheckbox("AutoBuyBNB", config.Config.AutoBuyBNB, func(checked bool) { config.Config.AutoBuyBNB = checked }).
		AddInputField("AutoBuyBNBQty", fmt.Sprintln(config.Config.AutoBuyBNBQty), 8, nil, func(text string) {
			num, err := strconv.ParseFloat(text, 64)
			if err != nil {
				// BUG:Add error output
				return
			}
			config.Config.AutoBuyBNBQty = num
		}).
		// Mode
		AddCheckbox("FOK", config.Config.IsFOK, func(checked bool) { config.Config.IsFOK = checked }).
		AddCheckbox("Future", config.Config.IsFuture, func(checked bool) { config.Config.IsFuture = checked }).
		AddCheckbox("OnlyMode1", config.Config.OnlyMode1, func(checked bool) { config.Config.OnlyMode1 = checked }).
		// Params
		AddInputField("MaxQty", fmt.Sprintln(config.Config.MaxQty), 8, nil, func(text string) {
			config.Config.MaxQty = text
		}).
		AddInputField("WaitDuration", fmt.Sprintln(config.Config.WaitDuration), 8, nil, func(text string) {
			num, err := strconv.ParseFloat(text, 64)
			if err != nil {
				// BUG:Add error output
				return
			}
			config.Config.WaitDuration = int64(num)
		}).
		AddInputField("CloseTimeOut", fmt.Sprintln(config.Config.CloseTimeOut), 8, nil, func(text string) {
			num, err := strconv.ParseFloat(text, 64)
			if err != nil {
				// BUG:Add error output
				return
			}
			config.Config.CloseTimeOut = int64(num)
		}).
		AddInputField("PauseClientTimeOutLimit", fmt.Sprintln(config.Config.PauseClientTimeOutLimit), 8, nil, func(text string) {
			num, err := strconv.ParseFloat(text, 64)
			if err != nil {
				// BUG:Add error output
				return
			}
			config.Config.PauseClientTimeOutLimit = int64(num)
		}).
		AddButton("Save", func() {
			t.Quit()
		}).
		AddButton("Quit", func() {
			t.app.Stop()
		})
	form.SetBorder(true).SetTitle("Config")
	return form
}

func (t *Tui) Quit() {
	t.closeCallBack()
	config.Save("../config.yaml")
}
