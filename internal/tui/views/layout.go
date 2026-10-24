package views

func visibleWindow(cursor, total, maxVisible int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	if maxVisible < 1 {
		maxVisible = 1
	}
	if maxVisible > total {
		maxVisible = total
	}
	start := cursor - maxVisible/2
	if start < 0 {
		start = 0
	}
	end := start + maxVisible
	if end > total {
		end = total
		start = max(0, end-maxVisible)
	}
	return start, end
}

func (m TesterModel) paneDimensions() (int, int) {
	halfWidth := (m.Width - 6) / 2
	if halfWidth < 35 {
		halfWidth = 40
	}
	paneHeight := m.Height - 13
	if paneHeight < 12 {
		paneHeight = 16
	}
	return halfWidth, paneHeight
}
