package dingding

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

type DingDingBotHook struct {
	infoBot  *DingDingBot
	errorBot *DingDingBot
}

func NewDingDingBotHook(
	logBotAccessToken, logBotSecrect,
	errorBotAccessToken, errorBotSecrect string,
	chLen int,
	interval int64,
) *DingDingBotHook {
	hook := &DingDingBotHook{
		infoBot:  NewDingDingBot(logBotAccessToken, logBotSecrect, chLen, interval),
		errorBot: NewDingDingBot(errorBotAccessToken, errorBotSecrect, chLen, interval),
	}

	go hook.infoBot.Start()
	go hook.errorBot.Start()

	return hook
}

func (hook *DingDingBotHook) Fire(entry *logrus.Entry) error {
	msg := fmt.Sprintf("\n%s \n%s\n", entry.Time.Format("2006-01-02 15:04:05.000"), entry.Message)
	switch entry.Level {
	case logrus.PanicLevel:
	case logrus.FatalLevel:
	case logrus.ErrorLevel:
		hook.errorBot.MsgCh <- msg
	case logrus.WarnLevel:
	case logrus.InfoLevel:
		hook.infoBot.MsgCh <- msg
	case logrus.DebugLevel:
	default:
		return nil
	}

	return nil
}

func (hook *DingDingBotHook) Levels() []logrus.Level {
	return logrus.AllLevels
}
