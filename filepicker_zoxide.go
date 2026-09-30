package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) clampFilePickerZoxideSelection() {
	n := len(m.app.FilePickerZoxidePaths)
	if n == 0 {
		m.app.FilePickerZoxideSelectedIndex = 0
		return
	}
	if m.app.FilePickerZoxideSelectedIndex >= n {
		m.app.FilePickerZoxideSelectedIndex = n - 1
	}
	if m.app.FilePickerZoxideSelectedIndex < 0 {
		m.app.FilePickerZoxideSelectedIndex = 0
	}
}

func (m Model) openFilePickerZoxide() (Model, tea.Cmd) {
	m.app.FilePickerZoxideMode = true
	m.app.FilePickerZoxideLoading = true
	m.app.FilePickerZoxideSelectedIndex = 0
	m.app.FilePickerZoxidePaths = nil
	m.app.FilePickerZoxideError = ""
	m.zoxideInput.SetValue("")
	m.zoxideInput.Focus()
	return m, loadZoxideDirsCmd("")
}

func (m Model) closeFilePickerZoxide() Model {
	m.app.FilePickerZoxideMode = false
	m.app.FilePickerZoxideLoading = false
	m.app.FilePickerZoxideError = ""
	m.zoxideInput.Blur()
	return m
}

func (m Model) handleFilePickerZoxideKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	k := m.app.Keys.FilePicker
	arrowDown := bind("down")
	arrowUp := bind("up")
	closeZoxide := bind("esc")

	switch {
	case pressed(msg, closeZoxide):
		return m.closeFilePickerZoxide(), nil
	case pressed(msg, arrowDown):
		if len(m.app.FilePickerZoxidePaths) > 0 {
			m.app.FilePickerZoxideSelectedIndex++
			m.clampFilePickerZoxideSelection()
		}
		return m, nil
	case pressed(msg, arrowUp):
		if len(m.app.FilePickerZoxidePaths) > 0 {
			m.app.FilePickerZoxideSelectedIndex--
			m.clampFilePickerZoxideSelection()
		}
		return m, nil
	case pressed(msg, k.Select):
		if len(m.app.FilePickerZoxidePaths) == 0 {
			return m, nil
		}
		path := m.app.FilePickerZoxidePaths[m.app.FilePickerZoxideSelectedIndex]
		return m.jumpFilePickerToDirectory(path)
	}

	oldVal := m.zoxideInput.Value()
	var cmd tea.Cmd
	m.zoxideInput, cmd = m.zoxideInput.Update(msg)
	if m.zoxideInput.Value() != oldVal {
		m.app.FilePickerZoxideLoading = true
		m.app.FilePickerZoxideSelectedIndex = 0
		return m, tea.Batch(cmd, loadZoxideDirsCmd(m.zoxideInput.Value()))
	}
	return m, cmd
}

func (m Model) jumpFilePickerToDirectory(path string) (Model, tea.Cmd) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		m.app.FilePickerZoxideError = "directory not found: " + path
		return m, nil
	}
	m = m.closeFilePickerZoxide()
	m.filepicker.CurrentDirectory = path
	_ = SaveFilepickerSettings(m.filepicker.SortBy.String(), m.filepicker.SortOrder.String(), m.filepicker.CurrentDirectory)
	var cmd tea.Cmd
	m.filepicker, cmd = m.filepicker.ScheduleReadDir()
	return m, cmd
}

func (m Model) renderFilePickerZoxidePopup(w, h int) string {
	dimStyle := lipgloss.NewStyle().Foreground(colDimGray)
	title := lipgloss.NewStyle().Foreground(colCyan).Bold(true).Render("Jump with zoxide")

	innerW := w - 4
	if innerW < 1 {
		innerW = 1
	}
	inputW := innerW - 2
	if inputW < 10 {
		inputW = 10
	}
	zi := m.zoxideInput
	zi.Width = inputW

	innerH := h - 4
	if innerH < 4 {
		innerH = 4
	}
	listH := innerH - 4
	if listH < 1 {
		listH = 1
	}

	var lines []string
	lines = append(lines, fitLine(title, innerW), "")
	lines = append(lines, fitLine(zi.View(), innerW), "")

	if m.app.FilePickerZoxideLoading {
		lines = append(lines, fitLine(dimStyle.Render("Loading…"), innerW))
	} else if m.app.FilePickerZoxideError != "" {
		errStyle := lipgloss.NewStyle().Foreground(colRed)
		lines = append(lines, fitLine(errStyle.Render(m.app.FilePickerZoxideError), innerW))
	} else if len(m.app.FilePickerZoxidePaths) == 0 {
		lines = append(lines, fitLine(dimStyle.Render("No matching directories"), innerW))
	} else {
		start := 0
		sel := m.app.FilePickerZoxideSelectedIndex
		if sel >= listH {
			start = sel - listH + 1
		}
		end := start + listH
		if end > len(m.app.FilePickerZoxidePaths) {
			end = len(m.app.FilePickerZoxidePaths)
		}
		for i := start; i < end; i++ {
			path := m.app.FilePickerZoxidePaths[i]
			line := path
			if i == sel {
				line = lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render("▸ " + path)
			} else {
				line = "  " + path
			}
			lines = append(lines, fitLine(line, innerW))
		}
	}

	for len(lines) < innerH-1 {
		lines = append(lines, "")
	}

	fpk := m.app.Keys.FilePicker
	footer := dimStyle.Italic(true).Render(fmt.Sprintf(
		"Type to filter • %s: move list • %s: go to directory • %s: back to browser",
		FormatKeys(bind("down"), " / ")+FormatKeys(bind("up"), ""),
		FormatKeys(fpk.Select, "/"),
		FormatKeys(bind("esc"), "/"),
	))
	lines = append(lines, fitLine(footer, innerW))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colYellow).
		Padding(1, 2).
		Width(w).Height(h).
		Render(strings.Join(lines, "\n"))
}
