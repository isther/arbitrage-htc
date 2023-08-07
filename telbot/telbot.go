package telbot

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/isther/arbitrage-htc/account"
	"github.com/isther/arbitrage-htc/config"
	"github.com/isther/arbitrage-htc/core"
	"github.com/isther/arbitrage-htc/utils"
	"github.com/mr-linch/go-tg"
	"github.com/mr-linch/go-tg/tgb"
	"github.com/mr-linch/go-tg/tgb/session"
	"github.com/sirupsen/logrus"
)

type TelBot struct {
	*tg.Client
	*tgb.Router
	tg.PeerID
	log bool

	ctx    context.Context
	cancel context.CancelFunc

	account.BalanceInfo
	core.TaskInfo
}

func NewTelBot(token string, balanceInfo account.BalanceInfo, taskInfoView core.TaskInfo) *TelBot {
	var ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt, os.Kill, syscall.SIGTERM)
	return &TelBot{
		Client:      tg.New(token),
		Router:      tgb.NewRouter(),
		PeerID:      nil,
		log:         false,
		ctx:         ctx,
		cancel:      cancel,
		BalanceInfo: balanceInfo,
		TaskInfo:    taskInfoView,
	}
}

func (t *TelBot) Run() {
	defer t.cancel()

	name, err := t.Client.GetMyName().Do(t.ctx)
	if err != nil {
		logrus.Error("Failed to get name: ", err)
		return
	}
	logrus.Infof("%s login successfully!", name.Name)

	// Clear commands
	t.Client.DeleteMyCommands().DoVoid(t.ctx)

	t.Client.SetMyCommands(
		[]tg.BotCommand{
			{Command: "log_enable", Description: "Enable log"},
			{Command: "log_disable", Description: "Disable log"},
			{Command: "account", Description: "Get account info"},
			{Command: "task", Description: "Get task info"},
			{Command: "settings", Description: "Setting"},
		}).DoVoid(t.ctx)

	t.
		AddStartHandler().
		AddEnableLogHandler().AddDisableLogHandler().
		AddAccountHandler().AddTaskHandler().
		AddSettingHandler()

	if err := tgb.NewPoller(
		t.Router,
		t.Client,
	).Run(t.ctx); err != nil {
		fmt.Println(err)
		defer os.Exit(1)
	}
	return
}

func (t *TelBot) AddStartHandler() *TelBot {
	t.Router.Message(
		func(ctx context.Context, msg *tgb.MessageUpdate) error {
			return msg.Answer(
				tg.HTML.Text(
					tg.HTML.Bold("👋 Hi, I'm echo bot!"),
					"",
					tg.HTML.Italic("🚀 Powered by", tg.HTML.Spoiler(tg.HTML.Link("Arbitrage-htc", "github.com/isther/arbitrage-htc"))),
				),
			).ParseMode(tg.HTML).DoVoid(ctx)
		},
		tgb.Command("start", tgb.WithCommandAlias("help")),
	)
	return t
}

func (t *TelBot) AddEnableLogHandler() *TelBot {
	t.Router.Message(
		func(ctx context.Context, msg *tgb.MessageUpdate) error {
			if t.PeerID == nil {
				t.PeerID = msg.Chat
				t.log = true
			}
			return msg.Answer("Log enabled!").DoVoid(ctx)
		},
		tgb.Command("log_enable"),
	)
	return t
}

func (t *TelBot) AddDisableLogHandler() *TelBot {
	t.Router.Message(
		func(ctx context.Context, msg *tgb.MessageUpdate) error {
			return msg.Answer("Log disabled!").DoVoid(ctx)
		},
		tgb.Command("log_disable"),
	)
	return t
}

func (t *TelBot) AddAccountHandler() *TelBot {
	t.Router.Message(
		func(ctx context.Context, msg *tgb.MessageUpdate) error {
			if err := msg.Update.Reply(ctx, msg.AnswerChatAction(tg.ChatActionUploadPhoto)); err != nil {
				logrus.Errorf("answer chat action: %v", err)
				return fmt.Errorf("answer chat action: %w", err)
			}

			var (
				content  string
				balances = t.BalanceInfo.BalanceInfo()
			)

			for k, v := range balances {
				content += fmt.Sprintf("%s: %s\n", k, v)
			}

			imgFilePath := filepath.Join("./imgs", fmt.Sprintf("account-%d-%d", msg.ID, time.Now().UTC().UnixMilli()))
			utils.CreatePNG(content, imgFilePath)

			inputFile, err := tg.NewInputFileLocal(imgFilePath)
			if err != nil {
				logrus.Error(err)
				return err
			}
			defer inputFile.Close()

			return msg.AnswerPhoto(
				tg.NewFileArgUpload(inputFile),
			).DoVoid(ctx)
		}, tgb.Command("account"),
	)
	return t
}

func (t *TelBot) AddTaskHandler() *TelBot {
	t.Router.Message(func(ctx context.Context, msg *tgb.MessageUpdate) error {
		if err := msg.Update.Reply(ctx, msg.AnswerChatAction(tg.ChatActionUploadPhoto)); err != nil {
			logrus.Errorf("answer chat action: %v", err)
			return fmt.Errorf("answer chat action: %w", err)
		}

		var (
			content string
			info    = t.TaskInfo.TaskInfo()
		)
		for _, line := range strings.Split(info, "|") {
			for k, word := range strings.Split(line, ":") {
				switch k {
				case 0:
					content += fmt.Sprintf("%s: ", word)
				case 1:
					content += fmt.Sprintf("%s\n", word)
				}
			}
		}

		imgFilePath := filepath.Join("./imgs", fmt.Sprintf("task-%d-%d", msg.ID, time.Now().UTC().UnixMilli()))
		utils.CreatePNG(content, imgFilePath)

		inputFile, err := tg.NewInputFileLocal(imgFilePath)
		if err != nil {
			logrus.Error(err)
			return err
		}
		defer inputFile.Close()

		return msg.AnswerPhoto(
			tg.NewFileArgUpload(inputFile),
		).DoVoid(ctx)
	},
		tgb.Command("task"),
	)
	return t
}

func (t *TelBot) AddSettingHandler() *TelBot {
	type SessionStep int8
	const (
		SessionStepInit SessionStep = iota
		SessionSelectSetting
		SessionStringInputting
		SessionBoolInputting
		SessionIntInputting
		SessionFloatInputting
	)

	type SettingType int8
	const (
		QtySetting SettingType = iota
		FOKSetting
		FutureSetting
		OnlyMode1Setting
		CycleNumberSetting
		WaitDurationSetting
		CloseTimeoutSetting
		Quit
	)

	type Session struct {
		Step  SessionStep
		value string
		SettingType
	}

	var (
		settingsMap = map[string]SettingType{
			"Qty":          QtySetting,
			"isFOK":        FOKSetting,
			"isFuture":     FutureSetting,
			"OnlyMode1":    OnlyMode1Setting,
			"CycleNumber":  CycleNumberSetting,
			"WaitDuration": WaitDurationSetting,
			"CloseTimeout": CloseTimeoutSetting,
			"Quit":         Quit,
		}

		settings = []string{
			"Qty",
			"isFOK",
			"isFuture",
			"OnlyMode1",
			"CycleNumber",
			"WaitDuration",
			"CloseTimeout",
			"Quit",
		}

		sessionManager = session.NewManager(Session{
			Step: SessionStepInit,
		}, session.WithStore(
			session.NewStoreFile("sessions"),
		))

		isSessionStep = func(state SessionStep) tgb.Filter {
			return sessionManager.Filter(func(session *Session) bool {
				return session.Step == state
			})
		}

		isDigit = tgb.Regexp(regexp.MustCompile(`^\d+$`))
		// isFloat = tgb.Regexp(regexp.MustCompile(`^(-?\d+)(\.\d+)?$`))
	)

	t.Router.
		Use(sessionManager).
		Message(
			func(ctx context.Context, msg *tgb.MessageUpdate) error {
				session := sessionManager.Get(ctx)
				session.Step = SessionSelectSetting

				buttonLayout := tg.NewButtonLayout[tg.KeyboardButton](3)

				for _, key := range settings {
					buttonLayout.Insert(tg.NewKeyboardButton(key))
				}

				return msg.Update.Reply(ctx, msg.Answer("Please input key: ").ReplyMarkup(
					tg.NewReplyKeyboardMarkup(
						buttonLayout.Keyboard()...,
					).WithResizeKeyboardMarkup(),
				))
			},
			tgb.Command("settings"),
		).
		// Select setting key
		Message(func(ctx context.Context, msg *tgb.MessageUpdate) error {
			session := sessionManager.Get(ctx)
			key := msg.Text

			value := settingsMap[key]
			if value == Quit {
				sessionManager.Reset(session)
				return msg.Update.Reply(
					ctx,
					msg.Answer("Bye").ReplyMarkup(tg.NewReplyKeyboardRemove()),
				)
			}

			switch value {
			case QtySetting:
				session.Step = SessionStringInputting
			case FOKSetting, FutureSetting, OnlyMode1Setting:
				session.Step = SessionBoolInputting
			case CycleNumberSetting, WaitDurationSetting, CloseTimeoutSetting:
				session.Step = SessionIntInputting
			}

			session.SettingType = value
			return msg.Update.Reply(
				ctx,
				msg.Answer("Please input value: ").
					ReplyMarkup(tg.NewReplyKeyboardRemove()),
			)
		}, isSessionStep(SessionSelectSetting), tgb.TextIn(settings)).
		Message(func(ctx context.Context, msg *tgb.MessageUpdate) error {
			return msg.Update.Reply(ctx, msg.Answer("Please, choose one of the buttons below 👇"))
		}, isSessionStep(SessionSelectSetting), tgb.Not(tgb.TextIn(settings))).
		// Input setting value type of int
		Message(func(ctx context.Context, msg *tgb.MessageUpdate) error {
			session := sessionManager.Get(ctx)
			value, err := strconv.ParseInt(msg.Text, 10, 64)
			if err != nil {
				logrus.Error(err)
				return fmt.Errorf("parse value: %w", err)
			}

			switch session.SettingType {
			case CycleNumberSetting:
				config.Config.CycleNumber = int(value)
			case WaitDurationSetting:
				config.Config.WaitDuration = value
			case CloseTimeoutSetting:
				config.Config.CloseTimeOut = value
			}

			sessionManager.Reset(session)
			return msg.Update.Reply(ctx, msg.Answer("Setting was saved"))
		}, isSessionStep(SessionIntInputting), isDigit).
		Message(func(ctx context.Context, msg *tgb.MessageUpdate) error {
			return msg.Update.Reply(ctx, msg.Answer("Please, send me just number"))
		}, isSessionStep(SessionIntInputting), tgb.Not(isDigit)).
		// Input setting value type of bool
		Message(func(ctx context.Context, msg *tgb.MessageUpdate) error {
			session := sessionManager.Get(ctx)
			value, err := strconv.ParseBool(msg.Text)
			if err != nil {
				logrus.Error(err)
				return fmt.Errorf("parse value: %w", err)
			}

			switch session.SettingType {
			case FOKSetting:
				config.Config.IsFOK = value
			case FutureSetting:
				config.Config.IsFuture = value
			case OnlyMode1Setting:
				config.Config.OnlyMode1 = value
			}

			sessionManager.Reset(session)
			return msg.Update.Reply(ctx, msg.Answer("Setting was saved"))
		}, isSessionStep(SessionBoolInputting), tgb.TextIn([]string{"true", "false"})).
		Message(func(ctx context.Context, msg *tgb.MessageUpdate) error {
			return msg.Update.Reply(ctx, msg.Answer("Please, send me just boolean value"))
		}, isSessionStep(SessionBoolInputting), tgb.Not(tgb.TextIn([]string{"true", "false"}))).
		// Input setting value type of bool
		Message(func(ctx context.Context, msg *tgb.MessageUpdate) error {
			session := sessionManager.Get(ctx)

			switch session.SettingType {
			case QtySetting:
				config.Config.MaxQty = msg.Text
			}

			sessionManager.Reset(session)
			return msg.Update.Reply(ctx, msg.Answer("Setting was saved"))
		}, isSessionStep(SessionStringInputting))

	return t
}

func (t *TelBot) Fire(entry *logrus.Entry) error {
	msg := fmt.Sprintf("\n%s \n%s\n", entry.Time.Format("2006-01-02 15:04:05.000"), entry.Message)
	if t.log {
		return t.Client.SendMessage(t.PeerID, msg).DoVoid(context.Background())
	}

	return nil
}

func (t *TelBot) Levels() []logrus.Level {
	return logrus.AllLevels
}
