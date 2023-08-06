package core

import (
	"sync/atomic"

	"github.com/sirupsen/logrus"
)

// Price
// AskPrice: Minimum sell pice | 卖1价格
// BidPrice: Maximum buy price | 买1价格
// Quantity
// AskQuantity: Minimum sell quantity | 卖1数量
// BidQuantity: Maximum buy quantity  | 买1数量

var (
	timeoutPauser = NewPause("Binance timeout")
	klinePauser   = NewPause("Binance Kline")
)

type Pauser struct {
	paused atomic.Bool // true: paused, false: unpaused
}

func NewPause(name string) *Pauser {
	pauser := &Pauser{
		paused: atomic.Bool{},
	}
	pauser.paused.Store(true)

	return pauser
}

func (p *Pauser) Pause(msg string) {
	if !p.paused.Load() {
		p.paused.Store(true)
		logrus.Error(msg)
	}
}

func (p *Pauser) UnPause(msg string) {
	if p.paused.Load() {
		p.paused.Store(false)
		logrus.Info(msg)
	}
}

func (p *Pauser) Value() bool {
	return p.paused.Load()
}
