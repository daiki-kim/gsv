package gsheets

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"gsv/internal/retry"
)

func init() { retry.BaseDelay = time.Millisecond }

func client(t *testing.T, h http.HandlerFunc) *APIClient {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	svc, err := sheets.NewService(context.Background(),
		option.WithEndpoint(srv.URL+"/"), option.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return New(svc)
}

func TestA1Range(t *testing.T) {
	tests := map[string]string{
		"Sheet1":   "'Sheet1'",
		"売上 2025":  "'売上 2025'",
		"It's Q1!": "'It''s Q1!'",
	}
	for in, want := range tests {
		if got := A1Range(in); got != want {
			t.Errorf("A1Range(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGetTabs(t *testing.T) {
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v4/spreadsheets/ID1") {
			t.Errorf("path = %s", r.URL.Path)
		}
		if f := r.URL.Query().Get("fields"); !strings.Contains(f, "sheets.properties") {
			t.Errorf("fields = %q", f)
		}
		w.Write([]byte(`{"properties":{"title":"売上管理"},"sheets":[
			{"properties":{"sheetId":0,"title":"Sheet1","gridProperties":{"rowCount":100,"columnCount":26}}},
			{"properties":{"sheetId":7,"title":"Sheet2"}}]}`))
	})
	title, tabs, err := c.GetTabs(context.Background(), "ID1")
	if err != nil {
		t.Fatal(err)
	}
	want := []Tab{{ID: 0, Title: "Sheet1", Rows: 100, Cols: 26}, {ID: 7, Title: "Sheet2"}}
	if title != "売上管理" || !reflect.DeepEqual(tabs, want) {
		t.Fatalf("title = %q, tabs = %+v", title, tabs)
	}
}

func TestGetValuesEscapesRangeAndStringifies(t *testing.T) {
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/values/'It''s Q1'") {
			t.Errorf("path = %s", r.URL.Path)
		}
		if v := r.URL.Query().Get("valueRenderOption"); v != "FORMATTED_VALUE" {
			t.Errorf("valueRenderOption = %q", v)
		}
		w.Write([]byte(`{"values":[["a",1,true],["b"]]}`))
	})
	rows, err := c.GetValues(context.Background(), "ID1", "It's Q1")
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"a", "1", "true"}, {"b"}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("rows = %v", rows)
	}
}

func TestGetValuesEmptyTab(t *testing.T) {
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"range":"Sheet1!A1:Z1000","majorDimension":"ROWS"}`))
	})
	rows, err := c.GetValues(context.Background(), "ID1", "Sheet1")
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows = %v, err = %v", rows, err)
	}
}

func TestRetriesOn429(t *testing.T) {
	calls := 0
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(429)
			w.Write([]byte(`{"error":{"code":429,"message":"slow down"}}`))
			return
		}
		w.Write([]byte(`{"values":[["ok"]]}`))
	})
	rows, err := c.GetValues(context.Background(), "ID1", "Sheet1")
	if err != nil || calls != 2 || rows[0][0] != "ok" {
		t.Fatalf("rows = %v, err = %v, calls = %d", rows, err, calls)
	}
}
