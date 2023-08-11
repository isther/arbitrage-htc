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
	"github.com/isther/arbitrage-htc/core"
	"github.com/isther/arbitrage-htc/utils"
	"github.com/mr-linch/go-tg"
	"github.com/mr-linch/go-tg/tgb"
	"github.com/mr-linch/go-tg/tgb/session"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type TelBot struct {
	*tg.Client
	*tgb.Router
	tg.PeerID
	log bool

	ctx    context.Context
	cancel context.CancelFunc

	account.BalanceInfo
	core.TaskControl
}

func NewTelBot(token string, balanceInfo account.BalanceInfo, taskInfoView core.TaskControl) *TelBot {
	var ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt, os.Kill, syscall.SIGTERM)
	return &TelBot{
		Client:      tg.New(token),
		Router:      tgb.NewRouter(),
		PeerID:      nil,
		log:         false,
		ctx:         ctx,
		cancel:      cancel,
		BalanceInfo: balanceInfo,
		TaskControl: taskInfoView,
	}
}

func (t *TelBot) Run() {
	defer t.cancel()

	name, err := t.Client.GetMyName().Do(t.ctx)
	if err != nil {
		panic("Failed to get name: " + err.Error())
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
			{Command: "tstart", Description: "Run task"},
			{Command: "tstop", Description: "Stop task"},
			{Command: "settings", Description: "Setting"},
		}).DoVoid(t.ctx)

	t.
		AddStartHandler().
		AddEnableLogHandler().AddDisableLogHandler().
		AddAccountHandler().AddTaskControlHandler().
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
				balances = t.BalanceInfo.GetBalanceInfo()
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

func (t *TelBot) AddTaskControlHandler() *TelBot {
	t.Router.Message(
		func(ctx context.Context, msg *tgb.MessageUpdate) error {
			t.TaskControl.Start()
			return msg.Answer("Task start!").DoVoid(ctx)
		},
		tgb.Command("tstart"),
	).Message(
		func(ctx context.Context, msg *tgb.MessageUpdate) error {
			t.TaskControl.Stop()
			return msg.Answer("Task stop!").DoVoid(ctx)
		},
		tgb.Command("tstop"),
	).Message(func(ctx context.Context, msg *tgb.MessageUpdate) error {
		if err := msg.Update.Reply(ctx, msg.AnswerChatAction(tg.ChatActionUploadPhoto)); err != nil {
			logrus.Errorf("answer chat action: %v", err)
			return fmt.Errorf("answer chat action: %w", err)
		}

		var (
			content string = "Task: \n"
			info           = t.TaskControl.GetInfo()
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

		addStringSetting := func(key string, unit string) {
			content += fmt.Sprintf("%s:%s%s\n", key, viper.GetString(key), unit)
		}

		addTimeSetting := func(key string) {
			content += fmt.Sprintf("%s:%dms\n", key, viper.GetDuration(key).Milliseconds())
		}

		addStringSetting("RatioMin", "")
		addStringSetting("RatioMax", "")
		addStringSetting("RatioProfit", "")
		addTimeSetting("CloseTimeout")
		addTimeSetting("WaitDuration")
		addStringSetting("MaxQty", "")
		content += "\nMode: \n"
		addStringSetting("FOK", "")
		addStringSetting("FOKStandard", "")
		addStringSetting("Future", "")
		addStringSetting("OnlyMode1", "")
		content += "\nFee: \n"
		addStringSetting("BNBMinQty", "")
		addStringSetting("AutoBuyBNB", "")
		addStringSetting("AutoBuyBNBQty", "")
		content += "\nPause: \n"
		addStringSetting("PauseMinKlineRatio", "")
		addStringSetting("PauseMaxKlineRatio", "")
		addStringSetting("PauseClientTimeOutLimit", "ms")

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
		CycleNumberSetting SettingType = iota
		UpdateBalanceSetting
		RatioMinSetting
		RatioMaxSetting
		RatioProfitSetting
		CloseTimeoutSetting
		WaitDurationSetting
		FOKSetting
		FutureSetting
		OnlyMode1Setting
		FOKStandardSetting
		PauseMaxKlineRatioSetting
		PauseMinKlineRatioSetting
		PauseClientTimeOutLimitSetting
		Quit
	)

	type Session struct {
		Step  SessionStep
		value string
		SettingType
	}

	var (
		settingsMap = map[string]SettingType{
			"CycleNumber":             CycleNumberSetting,
			"UpdateBalance":           UpdateBalanceSetting,
			"RatioMin":                RatioMinSetting,
			"RatioMax":                RatioMaxSetting,
			"RatioProfit":             RatioProfitSetting,
			"CloseTimeout":            CloseTimeoutSetting,
			"WaitDuration":            WaitDurationSetting,
			"FOK":                     FOKSetting,
			"FOKStandard":             FOKStandardSetting,
			"Future":                  FutureSetting,
			"OnlyMode1":               OnlyMode1Setting,
			"PauseMinKlineRatio":      PauseMinKlineRatioSetting,
			"PauseMaxKlineRatio":      PauseMaxKlineRatioSetting,
			"PauseClientTimeOutLimit": PauseClientTimeOutLimitSetting,
			"Quit":                    Quit,
		}

		settings = []string{
			"Quit",
			"CycleNumber",
			"Qty",
			"AutoAdjustQty",
			"UpdateBalance",
			"RatioMin",
			"RatioMax",
			"RatioProfit",
			"CloseTimeout",
			"WaitDuration",
			"FOKStandard",
			"Future",
			"OnlyMode1",
			"FOK",
			"PauseMinKlineRatio",
			"PauseMaxKlineRatio",
			"PauseClientTimeOutLimit",
		}

		sessionManager = session.NewManager(Session{
			Step: SessionStepInit,
		})

		isSessionStep = func(state SessionStep) tgb.Filter {
			return sessionManager.Filter(func(session *Session) bool {
				return session.Step == state
			})
		}

		isDigit = tgb.Regexp(regexp.MustCompile(`^\d+$`))
		isFloat = tgb.Regexp(regexp.MustCompile(`^(-?\d+)(\.\d+)?$`))
	)

	t.Router.
		Use(sessionManager).
		Message(
			func(ctx context.Context, msg *tgb.MessageUpdate) error {
				session := sessionManager.Get(ctx)
				session.Step = SessionSelectSetting

				buttonLayout := tg.NewButtonLayout[tg.KeyboardButton](4)
				buttonLayout.Row(
					tg.NewKeyboardButton("Quit"),
					tg.NewKeyboardButton("CycleNumber"),
					tg.NewKeyboardButton("UpdateBalance"),
				)

				buttonLayout.Row(
					tg.NewKeyboardButton("FOK"),
					tg.NewKeyboardButton("FOKStandard"),
					tg.NewKeyboardButton("Future"),
					tg.NewKeyboardButton("OnlyMode1"),
				)

				buttonLayout.Row(
					tg.NewKeyboardButton("RatioMin"),
					tg.NewKeyboardButton("RatioMax"),
					tg.NewKeyboardButton("RatioProfit"),
				)

				buttonLayout.Row(
					tg.NewKeyboardButton("CloseTimeout"),
					tg.NewKeyboardButton("WaitDuration"),
				)

				buttonLayout.Row(
					tg.NewKeyboardButton("PauseMinKlineRatio"),
					tg.NewKeyboardButton("PauseMaxKlineRatio"),
					tg.NewKeyboardButton("PauseClientTimeOutLimit"),
				)

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
			case FOKSetting, FutureSetting, OnlyMode1Setting:
				session.Step = SessionBoolInputting
			case CycleNumberSetting, WaitDurationSetting, CloseTimeoutSetting, PauseClientTimeOutLimitSetting:
				session.Step = SessionIntInputting
			case RatioMinSetting, RatioMaxSetting, RatioProfitSetting, FOKStandardSetting, PauseMinKlineRatioSetting, PauseMaxKlineRatioSetting:
				session.Step = SessionFloatInputting
			case UpdateBalanceSetting:
				t.TaskControl.UpdateBalance()
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
		// Input setting value type of string
		Message(func(ctx context.Context, msg *tgb.MessageUpdate) error {
			session := sessionManager.Get(ctx)

			sessionManager.Reset(session)
			return msg.Update.Reply(ctx, msg.Answer("Setting was saved"))
		}, isSessionStep(SessionStringInputting)).
		Message(func(ctx context.Context, msg *tgb.MessageUpdate) error {
			return msg.Update.Reply(ctx, msg.Answer("Please, send me just float value"))
		}, isSessionStep(SessionStringInputting), tgb.Not(isFloat)).
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
				viper.Set("CycleNumber", int(value))
				t.TaskControl.CycleNumber()
			case WaitDurationSetting:
				viper.Set("WaitDuration", time.Duration(value)*time.Millisecond)
				t.TaskControl.WaitDuration()
			case CloseTimeoutSetting:
				viper.Set("CloseTimeOut", time.Duration(value)*time.Millisecond)
				t.TaskControl.CloseTimeOut()
			case PauseClientTimeOutLimitSetting:
				viper.Set("PauseClientTimeOutLimit", value)
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
				viper.Set("FOK", value)
				t.TaskControl.FOK()
			case FutureSetting:
				viper.Set("Future", value)
				t.TaskControl.Future()
			case OnlyMode1Setting:
				viper.Set("OnlyMode1", value)
				t.TaskControl.OnlyMode1()
			}

			sessionManager.Reset(session)
			return msg.Update.Reply(ctx, msg.Answer("Setting was saved"))
		}, isSessionStep(SessionBoolInputting), tgb.TextIn([]string{"true", "false"})).
		Message(func(ctx context.Context, msg *tgb.MessageUpdate) error {
			return msg.Update.Reply(ctx, msg.Answer("Please, send me just boolean value"))
		}, isSessionStep(SessionBoolInputting), tgb.Not(tgb.TextIn([]string{"true", "false"}))).
		// Input setting value type of float
		Message(func(ctx context.Context, msg *tgb.MessageUpdate) error {
			session := sessionManager.Get(ctx)
			value, err := strconv.ParseFloat(msg.Text, 64)
			if err != nil {
				logrus.Error(err)
				return fmt.Errorf("parse value: %w", err)
			}

			switch session.SettingType {
			case RatioMinSetting:
				viper.Set("RatioMin", decimal.NewFromFloat(value))
				t.TaskControl.RatioMin()
			case RatioMaxSetting:
				viper.Set("RatioMax", decimal.NewFromFloat(value))
				t.TaskControl.RatioMax()
			case RatioProfitSetting:
				viper.Set("RatioProfit", decimal.NewFromFloat(value))
				t.TaskControl.RatioProfit()
			case FOKStandardSetting:
				viper.Set("FOKStandard", decimal.NewFromFloat(value))
				t.TaskControl.FOKStandard()
			case PauseMinKlineRatioSetting:
				viper.Set("PauseMinKlineRatio", decimal.NewFromFloat(value))
			case PauseMaxKlineRatioSetting:
				viper.Set("PauseMaxKlineRatio", decimal.NewFromFloat(value))
			}

			sessionManager.Reset(session)
			return msg.Update.Reply(ctx, msg.Answer("Setting was saved"))
		}, isSessionStep(SessionFloatInputting), isFloat).
		Message(func(ctx context.Context, msg *tgb.MessageUpdate) error {
			return msg.Update.Reply(ctx, msg.Answer("Please, send me just float value"))
		}, isSessionStep(SessionFloatInputting), tgb.Not(isFloat))

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
