// Package gdrive lists folders and spreadsheets from Google Drive.
package gdrive

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/drive/v3"

	"gsv/internal/retry"
)

type Kind int

const (
	KindFolder Kind = iota
	KindSpreadsheet
)

type Item struct {
	ID   string
	Name string
	Kind Kind
}

type Client interface {
	ListChildren(ctx context.Context, parentID string) ([]Item, error)
	ListSharedWithMe(ctx context.Context) ([]Item, error)
	SearchSpreadsheets(ctx context.Context, name string) ([]Item, error)
}

const (
	mimeFolder   = "application/vnd.google-apps.folder"
	mimeSheet    = "application/vnd.google-apps.spreadsheet"
	mimeShortcut = "application/vnd.google-apps.shortcut"
)

var baseFilter = fmt.Sprintf("trashed = false and (mimeType = '%s' or mimeType = '%s' or mimeType = '%s')",
	mimeFolder, mimeSheet, mimeShortcut)

func escape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `'`, `\'`)
}

func ChildrenQuery(parentID string) string {
	return fmt.Sprintf("'%s' in parents and %s", escape(parentID), baseFilter)
}

func SharedQuery() string { return "sharedWithMe = true and " + baseFilter }

func SearchQuery(name string) string {
	return fmt.Sprintf("name contains '%s' and mimeType = '%s' and trashed = false", escape(name), mimeSheet)
}

type APIClient struct {
	svc *drive.Service
}

func New(svc *drive.Service) *APIClient { return &APIClient{svc: svc} }

func (c *APIClient) ListChildren(ctx context.Context, parentID string) ([]Item, error) {
	return c.list(ctx, ChildrenQuery(parentID))
}

func (c *APIClient) ListSharedWithMe(ctx context.Context) ([]Item, error) {
	return c.list(ctx, SharedQuery())
}

func (c *APIClient) SearchSpreadsheets(ctx context.Context, name string) ([]Item, error) {
	return c.list(ctx, SearchQuery(name))
}

func (c *APIClient) list(ctx context.Context, q string) ([]Item, error) {
	var items []Item
	token := ""
	for {
		var fl *drive.FileList
		err := retry.Do(ctx, func() error {
			call := c.svc.Files.List().Q(q).
				OrderBy("folder,name").PageSize(1000).
				SupportsAllDrives(true).IncludeItemsFromAllDrives(true).Corpora("allDrives").
				Fields("nextPageToken, files(id, name, mimeType, shortcutDetails)").
				Context(ctx)
			if token != "" {
				call = call.PageToken(token)
			}
			var err error
			fl, err = call.Do()
			return err
		})
		if err != nil {
			return nil, err
		}
		for _, f := range fl.Files {
			if it, ok := toItem(f); ok {
				items = append(items, it)
			}
		}
		if fl.NextPageToken == "" {
			break
		}
		token = fl.NextPageToken
	}
	// Shortcuts to folders sort among files on the API side; put every folder first.
	sort.SliceStable(items, func(i, j int) bool { return items[i].Kind < items[j].Kind })
	return items, nil
}

func toItem(f *drive.File) (Item, bool) {
	id, mime := f.Id, f.MimeType
	if mime == mimeShortcut {
		if f.ShortcutDetails == nil {
			return Item{}, false
		}
		id, mime = f.ShortcutDetails.TargetId, f.ShortcutDetails.TargetMimeType
	}
	switch mime {
	case mimeFolder:
		return Item{ID: id, Name: f.Name, Kind: KindFolder}, true
	case mimeSheet:
		return Item{ID: id, Name: f.Name, Kind: KindSpreadsheet}, true
	}
	return Item{}, false
}
