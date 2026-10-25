package views

import "testing"

func TestVisibleWindow(t *testing.T) {
	tests := []struct {
		name       string
		cursor     int
		total      int
		maxVisible int
		wantStart  int
		wantEnd    int
	}{
		{name: "empty list", cursor: 0, total: 0, maxVisible: 10, wantStart: 0, wantEnd: 0},
		{name: "list shorter than window", cursor: 2, total: 5, maxVisible: 10, wantStart: 0, wantEnd: 5},
		{name: "cursor at start", cursor: 0, total: 100, maxVisible: 10, wantStart: 0, wantEnd: 10},
		{name: "cursor in middle", cursor: 50, total: 100, maxVisible: 10, wantStart: 45, wantEnd: 55},
		{name: "cursor at end", cursor: 99, total: 100, maxVisible: 10, wantStart: 90, wantEnd: 100},
		{name: "cursor beyond list", cursor: 200, total: 5, maxVisible: 3, wantStart: 2, wantEnd: 5},
		{name: "zero visible size clamps to one", cursor: 0, total: 5, maxVisible: 0, wantStart: 0, wantEnd: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := visibleWindow(tt.cursor, tt.total, tt.maxVisible)
			if start != tt.wantStart || end != tt.wantEnd {
				t.Fatalf("visibleWindow(%d, %d, %d) = (%d, %d), want (%d, %d)",
					tt.cursor, tt.total, tt.maxVisible, start, end, tt.wantStart, tt.wantEnd)
			}
		})
	}
}

func TestPaneDimensions(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		height     int
		wantWidth  int
		wantHeight int
	}{
		{name: "minimums", width: 0, height: 0, wantWidth: 40, wantHeight: 16},
		{name: "narrow", width: 50, height: 30, wantWidth: 40, wantHeight: 17},
		{name: "wide", width: 100, height: 40, wantWidth: 47, wantHeight: 27},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := TesterModel{Width: tt.width, Height: tt.height}
			width, height := m.paneDimensions()
			if width != tt.wantWidth || height != tt.wantHeight {
				t.Fatalf("paneDimensions() = (%d, %d), want (%d, %d)", width, height, tt.wantWidth, tt.wantHeight)
			}
		})
	}
}
