package tui

import (
	"strings"

	"github.com/isther/arbitrage-htc/core"
	"github.com/rivo/tview"
)

type TuiTaskInfo struct {
	info          *tview.Table
	taskInfoViews []core.TaskInfoView
}

func NewTuiTaskInfo(taskInfoView []core.TaskInfoView) *TuiTaskInfo {
	info := tview.NewTable()
	info.
		SetSelectable(false, false).
		SetSeparator(tview.Borders.Vertical).
		SetTitle("Task").SetBorder(true)

	info.SetCell(0, 0, tview.NewTableCell(""))
	return &TuiTaskInfo{
		info:          info,
		taskInfoViews: taskInfoView,
	}
}

func (t *TuiTaskInfo) Item() *tview.Table {
	return t.info
}

func (t *TuiTaskInfo) Update() {
	for _, taskInfoView := range t.taskInfoViews {
		info := taskInfoView.TaskInfo()
		for i, word := range strings.Split(info, "|") {
			for k, v := range strings.Split(word, ":") {
				t.info.SetCell(k, i, tview.NewTableCell(v).SetExpansion(1).SetAlign(tview.AlignCenter))
			}
		}
	}
}
