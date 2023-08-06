package tui

import (
	"fmt"

	"github.com/rivo/tview"

	binancesdk "github.com/adshao/go-binance/v2"
)

type TuiBookTickerUpdater struct {
	table *tview.Table

	symbols []string
}

func NewTuiBookTickerUpdater(
	symbols []string,
) *TuiBookTickerUpdater {
	table := tview.NewTable()
	table.
		SetSelectable(false, false).
		SetSeparator(tview.Borders.Vertical).
		SetTitle("BookTicker").SetBorder(true)

	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			switch j {
			case 0:
				table.SetCell(i, j, tview.NewTableCell(fmt.Sprintf("[yellow]%s", symbols[i])).SetExpansion(1).SetAlign(tview.AlignCenter))
			case 1:
				table.SetCell(i, j, tview.NewTableCell(fmt.Sprintf("[red]0")).SetExpansion(1).SetAlign(tview.AlignCenter))
			case 2:
				table.SetCell(i, j, tview.NewTableCell(fmt.Sprintf("[green]0")).SetExpansion(1).SetAlign(tview.AlignCenter))
			}
		}
	}

	return &TuiBookTickerUpdater{
		table:   table,
		symbols: symbols,
	}
}

func (t *TuiBookTickerUpdater) Item() *tview.Table {
	return t.table
}

func (t *TuiBookTickerUpdater) UpdateBookTickerEvent(event *binancesdk.WsBookTickerEvent) {
	var (
		i int
	)
	switch event.Symbol {
	case t.symbols[0]:
		i = 0
	case t.symbols[1]:
		i = 1
	case t.symbols[2]:
		i = 2
	default:
		return
	}

	t.table.SetCell(i, 1, tview.NewTableCell(fmt.Sprintf("[red]%s", event.BestAskPrice)).SetExpansion(1).SetAlign(tview.AlignCenter))
	t.table.SetCell(i, 2, tview.NewTableCell(fmt.Sprintf("[green]%s", event.BestBidPrice)).SetExpansion(1).SetAlign(tview.AlignCenter))
}
