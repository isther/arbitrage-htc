package telbot

import (
	"fmt"
	"strings"
	"time"

	"github.com/isther/arbitrage-htc/account"
	"github.com/isther/arbitrage-htc/core"
	"github.com/sirupsen/logrus"
	tele "gopkg.in/telebot.v3"
)

type TelBot struct {
	*tele.Bot
	*tele.User
	log bool

	account.BalanceInfo
	core.TaskInfo
}

func NewTelBot(token string, balanceInfo account.BalanceInfo, taskInfoView core.TaskInfo) *TelBot {
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}
	logrus.Info(token)

	b, err := tele.NewBot(pref)
	if err != nil {
		logrus.Error(err)
		return nil
	}

	return &TelBot{b, nil, false, balanceInfo, taskInfoView}
}

func (t *TelBot) Run() {
	// account - Get account info
	// task - Get task info
	// log_enable - Enable log
	// log_disable - Disable log
	t.Bot.SetCommands([]tele.Command{
		{Text: "/account", Description: "Get account info"},
		{Text: "/task", Description: "Get task info"},
		{Text: "/log_enable", Description: "Enable log"},
		{Text: "/log_disable", Description: "Disable log"},
	})

	t.Bot.Handle("/start", func(c tele.Context) error {
		c.Send("Hello!")
		return nil
	})

	t.Bot.Handle("/account", func(c tele.Context) error {
		var (
			msg      = "--Account--\n"
			balances = t.BalanceInfo.BalanceInfo()
		)

		for k, v := range balances {
			msg += fmt.Sprintf("%s:%s\n", k, v)
		}
		c.Send(msg)
		return nil
	})

	t.Bot.Handle("/task", func(c tele.Context) error {
		var (
			msg  = "--Task--\n"
			info = t.TaskInfo.TaskInfo()
		)

		for _, word := range strings.Split(info, "|") {
			msg += fmt.Sprintf("%s\n", word)
		}
		c.Send(msg)
		return nil
	})

	t.Bot.Handle("/log_enable", func(c tele.Context) error {
		if t.User == nil {
			t.User = c.Sender()
		}

		t.log = true
		c.Send("Log Enabled")
		return nil
	})

	t.Bot.Handle("/log_disable", func(c tele.Context) error {
		t.log = false
		c.Send("Log Disable")
		return nil
	})

	t.Bot.Handle(tele.OnText, func(c tele.Context) error {
		c.Send("Not Found")
		return nil
	})

	t.Bot.Start()
}

func (t *TelBot) Fire(entry *logrus.Entry) error {
	msg := fmt.Sprintf("\n%s \n%s\n", entry.Time.Format("2006-01-02 15:04:05.000"), entry.Message)
	if t.log {
		t.Bot.Send(t.User, msg)
	}

	return nil
}

func (t *TelBot) Levels() []logrus.Level {
	return logrus.AllLevels
}
