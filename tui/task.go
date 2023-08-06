package tui

import (
	"fmt"
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
	for taskNum, taskInfoView := range t.taskInfoViews {
		info := taskInfoView.TaskInfo()
		t.info.SetCell(1, taskNum, tview.NewTableCell(fmt.Sprintf("Task%v", taskNum)).SetExpansion(1).SetAlign(tview.AlignCenter))
		for i, word := range strings.Split(info, "|") {
			for k, v := range strings.Split(word, ":") {
				t.info.SetCell(k, i+1, tview.NewTableCell(v).SetExpansion(1).SetAlign(tview.AlignCenter))
			}
		}
	}
}
