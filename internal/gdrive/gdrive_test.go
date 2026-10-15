package gdrive

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const filter = "trashed = false and (mimeType = 'application/vnd.google-apps.folder' or " +
	"mimeType = 'application/vnd.google-apps.spreadsheet' or mimeType = 'application/vnd.google-apps.shortcut')"

func TestQueries(t *testing.T) {
	if got, want := ChildrenQuery("abc"), "'abc' in parents and "+filter; got != want {
		t.Errorf("ChildrenQuery = %q", got)
	}
	if got, want := SharedQuery(), "sharedWithMe = true and "+filter; got != want {
		t.Errorf("SharedQuery = %q", got)
	}
	want := `name contains 'O\'Brien\\x' and mimeType = 'application/vnd.google-apps.spreadsheet' and trashed = false`
	if got := SearchQuery(`O'Brien\x`); got != want {
		t.Errorf("SearchQuery = %q, want %q", got, want)
	}
}

func client(t *testing.T, h http.HandlerFunc) *APIClient {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	svc, err := drive.NewService(context.Background(),
		option.WithEndpoint(srv.URL+"/"), option.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return New(svc)
}

func TestListChildrenPagesSortsAndResolvesShortcuts(t *testing.T) {
	var queries []string
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		queries = append(queries, q.Get("q"))
		for k, v := range map[string]string{
			"supportsAllDrives": "true", "includeItemsFromAllDrives": "true",
			"corpora": "allDrives", "pageSize": "1000", "orderBy": "folder,name",
		} {
			if q.Get(k) != v {
				t.Errorf("%s = %q, want %q", k, q.Get(k), v)
			}
		}
		if q.Get("pageToken") == "" {
			w.Write([]byte(`{"nextPageToken":"P2","files":[
				{"id":"s1","name":"予算表","mimeType":"application/vnd.google-apps.spreadsheet"},
				{"id":"x1","name":"to-folder","mimeType":"application/vnd.google-apps.shortcut",
				 "shortcutDetails":{"targetId":"f9","targetMimeType":"application/vnd.google-apps.folder"}}]}`))
			return
		}
		w.Write([]byte(`{"files":[
			{"id":"f1","name":"2025","mimeType":"application/vnd.google-apps.folder"},
			{"id":"x2","name":"to-doc","mimeType":"application/vnd.google-apps.shortcut",
			 "shortcutDetails":{"targetId":"d1","targetMimeType":"application/vnd.google-apps.document"}}]}`))
	})
	items, err := c.ListChildren(context.Background(), "root")
	if err != nil {
		t.Fatal(err)
	}
	want := []Item{
		{ID: "f9", Name: "to-folder", Kind: KindFolder},
		{ID: "f1", Name: "2025", Kind: KindFolder},
		{ID: "s1", Name: "予算表", Kind: KindSpreadsheet},
	}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("items = %+v", items)
	}
	if len(queries) != 2 || queries[0] != ChildrenQuery("root") {
		t.Fatalf("queries = %q", queries)
	}
}

func TestListSharedWithMeAndSearchUseTheirQueries(t *testing.T) {
	var got string
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query().Get("q")
		w.Write([]byte(`{"files":[]}`))
	})
	if _, err := c.ListSharedWithMe(context.Background()); err != nil || got != SharedQuery() {
		t.Fatalf("q = %q, err = %v", got, err)
	}
	if _, err := c.SearchSpreadsheets(context.Background(), "売上"); err != nil || got != SearchQuery("売上") {
		t.Fatalf("q = %q, err = %v", got, err)
	}
}

func TestListReturnsAPIError(t *testing.T) {
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"error":{"code":403,"message":"forbidden"}}`))
	})
	if _, err := c.ListChildren(context.Background(), "root"); err == nil {
		t.Fatal("want error")
	}
}
