package dingding

import (
	"context"
	"time"

	"github.com/CatchZeng/dingtalk/pkg/dingtalk"
)

type DingDingBot struct {
	client   *dingtalk.Client
	MsgCh    chan string
	interval int64
}

func NewDingDingBot(accessToken, secrect string, chLen int, interval int64) *DingDingBot {
	return &DingDingBot{
		client: dingtalk.NewClient(accessToken, secrect),
		MsgCh:  make(chan string, chLen),
	}
}

func (d *DingDingBot) Start() {
	var (
		msg         string
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(d.interval)*time.Millisecond)
	)
	defer cancel()

	for {
		select {
		case m := <-d.MsgCh:
			msg += m
		case <-ctx.Done():
			if msg != "" {
				d.client.Send(dingtalk.NewTextMessage().SetContent(msg))
				msg = ""
			}
			cancel()
			ctx, cancel = context.WithTimeout(context.Background(), time.Duration(d.interval)*time.Millisecond)
		}
		time.Sleep(time.Millisecond * 100)
	}
}
