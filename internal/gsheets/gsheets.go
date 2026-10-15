// Package gsheets reads spreadsheet tabs and values from the Sheets API.
package gsheets

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/sheets/v4"

	"gsv/internal/retry"
)

type Tab struct {
	ID    int64
	Title string
	Rows  int64
	Cols  int64
}

type Client interface {
	GetTabs(ctx context.Context, spreadsheetID string) (title string, tabs []Tab, err error)
	GetValues(ctx context.Context, spreadsheetID, tabTitle string) ([][]string, error)
}

type APIClient struct {
	svc *sheets.Service
}

func New(svc *sheets.Service) *APIClient { return &APIClient{svc: svc} }

// A1Range quotes a tab title so it can be used as a whole-sheet A1 range.
func A1Range(tab string) string {
	return "'" + strings.ReplaceAll(tab, "'", "''") + "'"
}

func (c *APIClient) GetTabs(ctx context.Context, id string) (string, []Tab, error) {
	var ss *sheets.Spreadsheet
	err := retry.Do(ctx, func() error {
		var err error
		ss, err = c.svc.Spreadsheets.Get(id).
			Fields("properties.title,sheets.properties(sheetId,title,gridProperties)").
			Context(ctx).Do()
		return err
	})
	if err != nil {
		return "", nil, err
	}
	var tabs []Tab
	for _, s := range ss.Sheets {
		p := s.Properties
		if p == nil {
			continue
		}
		t := Tab{ID: p.SheetId, Title: p.Title}
		if g := p.GridProperties; g != nil {
			t.Rows, t.Cols = g.RowCount, g.ColumnCount
		}
		tabs = append(tabs, t)
	}
	title := ""
	if ss.Properties != nil {
		title = ss.Properties.Title
	}
	return title, tabs, nil
}

func (c *APIClient) GetValues(ctx context.Context, id, tab string) ([][]string, error) {
	var vr *sheets.ValueRange
	err := retry.Do(ctx, func() error {
		var err error
		vr, err = c.svc.Spreadsheets.Values.Get(id, A1Range(tab)).
			ValueRenderOption("FORMATTED_VALUE").
			Context(ctx).Do()
		return err
	})
	if err != nil {
		return nil, err
	}
	rows := make([][]string, len(vr.Values))
	for i, row := range vr.Values {
		rows[i] = make([]string, len(row))
		for j, cell := range row {
			if cell != nil {
				rows[i][j] = fmt.Sprint(cell)
			}
		}
	}
	return rows, nil
}
