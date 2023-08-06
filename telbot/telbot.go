package telbot

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/isther/arbitrage-htc/account"
	"github.com/isther/arbitrage-htc/core"
	"github.com/isther/arbitrage-htc/utils"
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
			msg      string
			balances = t.BalanceInfo.BalanceInfo()
		)

		for k, v := range balances {
			msg += fmt.Sprintf("%s: %s\n", k, v)
		}
		imgFilePath := filepath.Join("./imgs", fmt.Sprintf("account%d-%d", c.Chat().ID, time.Now().UTC().UnixMilli()))
		utils.CreatePNG(msg, imgFilePath)
		p := &tele.Photo{
			File: tele.FromDisk(imgFilePath),
		}
		c.SendAlbum(tele.Album{p})
		return nil
	})

	t.Bot.Handle("/task", func(c tele.Context) error {
		var (
			msg  string
			info = t.TaskInfo.TaskInfo()
		)
		for _, line := range strings.Split(info, "|") {
			for k, word := range strings.Split(line, ":") {
				switch k {
				case 0:
					msg += fmt.Sprintf("%s: ", word)
				case 1:
					msg += fmt.Sprintf("%s\n", word)
				}
			}
		}

		imgFilePath := filepath.Join("./imgs", fmt.Sprintf("task%d-%d", c.Chat().ID, time.Now().UTC().UnixMilli()))
		utils.CreatePNG(msg, imgFilePath)
		p := &tele.Photo{
			File: tele.FromDisk(imgFilePath),
		}
		c.SendAlbum(tele.Album{p})
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
