package tui

import (
	"github.com/rivo/tview"
)

type TuiBalance struct {
	table *tview.Table
}

func NewTuiBalance() *TuiBalance {
	table := tview.NewTable()
	table.
		SetSelectable(false, false).
		SetSeparator(tview.Borders.Vertical).
		SetTitle("Account").SetBorder(true)

	return &TuiBalance{
		table: table,
	}
}

func (t *TuiBalance) View(balances map[string]string) {
	var cnt = 0
	for k, v := range balances {
		t.table.SetCell(cnt, 0, tview.NewTableCell(k).SetExpansion(1).SetAlign(tview.AlignCenter))
		t.table.SetCell(cnt, 1, tview.NewTableCell(v).SetExpansion(1).SetAlign(tview.AlignCenter))
		cnt++
	}
}

func (t *TuiBalance) Item() *tview.Table {
	return t.table
}
