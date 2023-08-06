package tui

import (
	"strings"

	"github.com/isther/arbitrage-htc/core"
	"github.com/rivo/tview"
)

type TuiTaskInfo struct {
	info      *tview.Table
	taskInfos []core.TaskInfo
}

func NewTuiTaskInfo(taskInfoView []core.TaskInfo) *TuiTaskInfo {
	info := tview.NewTable()
	info.
		SetSelectable(false, false).
		SetSeparator(tview.Borders.Vertical).
		SetTitle("Task").SetBorder(true)

	info.SetCell(0, 0, tview.NewTableCell(""))
	return &TuiTaskInfo{
		info:      info,
		taskInfos: taskInfoView,
	}
}

func (t *TuiTaskInfo) Item() *tview.Table {
	return t.info
}

func (t *TuiTaskInfo) Update() {
	for _, taskInfoView := range t.taskInfos {
		info := taskInfoView.TaskInfo()
		for i, word := range strings.Split(info, "|") {
			for k, v := range strings.Split(word, ":") {
				t.info.SetCell(k, i, tview.NewTableCell(v).SetExpansion(1).SetAlign(tview.AlignCenter))
			}
		}
	}
}
