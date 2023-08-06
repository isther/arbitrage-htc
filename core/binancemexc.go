package core

import (
	"github.com/shopspring/decimal"
)

var (
	klineRatioBase = decimal.NewFromInt(10000)
	base           = decimal.NewFromInt(10000)
)

type TaskStatus int

var (
	INIT         TaskStatus = 0
	PROCESSOPEN  TaskStatus = 1
	PROCESSCLOSE TaskStatus = 2
	FILLED       TaskStatus = 3
	UNFILLED     TaskStatus = 4
)

func (t TaskStatus) String() string {
	switch t {
	case INIT:
		return "INIT"
	case PROCESSOPEN:
		return "PROCESSOPEN"
	case PROCESSCLOSE:
		return "PROCESSCLOSE"
	case FILLED:
		return "FILLED"
	case UNFILLED:
		return "UNFILLED"
	default:
		return "UNKNOWN"
	}
}

/*
							==> UNFILLED										==> END
INIT	==>	PROCESSOPEN		==> FILLED		==> PROCESSCLOSE	==> FILLED		==> END
																==> UNFILLED	==> END
*/
