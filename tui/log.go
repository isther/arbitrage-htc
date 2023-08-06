package tui

import (
	"fmt"

	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
)

type TuiLogger struct {
	logContent *tview.TextView
}

func NewTuiLogger() *TuiLogger {
	logContent := tview.NewTextView()
	logContent.SetDynamicColors(true).SetScrollable(true)
	logContent.SetMaxLines(200).SetTitle("Log").SetBorder(true)
	return &TuiLogger{
		logContent: logContent,
	}
}

func (l *TuiLogger) Item() *tview.TextView {
	return l.logContent
}

func (l *TuiLogger) Fire(entry *logrus.Entry) error {
	var msg string
	switch entry.Level {
	case logrus.InfoLevel:
		msg = fmt.Sprintf("[blue][%s][white] %s", entry.Time.Format("2006-01-02 15:04:05.000"), entry.Message)
	case logrus.ErrorLevel:
		msg = fmt.Sprintf("[red][%s][white] %s", entry.Time.Format("2006-01-02 15:04:05.000"), entry.Message)
	default:
		msg = fmt.Sprintf("[green][%s][white] %s", entry.Time.Format("2006-01-02 15:04:05.000"), entry.Message)
	}

	l.logContent.SetText(l.logContent.GetText(false) + msg)

	return nil
}

func (l *TuiLogger) Levels() []logrus.Level {
	return logrus.AllLevels
}
