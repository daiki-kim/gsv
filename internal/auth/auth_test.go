package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestLoadOAuthConfigMissing(t *testing.T) {
	_, err := LoadOAuthConfig(filepath.Join(t.TempDir(), "credentials.json"))
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadOAuthConfigInstalledApp(t *testing.T) {
	p := filepath.Join(t.TempDir(), "credentials.json")
	os.WriteFile(p, []byte(`{"installed":{"client_id":"cid","client_secret":"sec",
		"auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token",
		"redirect_uris":["http://localhost"]}}`), 0o600)
	cfg, err := LoadOAuthConfig(p)
	if err != nil || cfg.ClientID != "cid" || len(cfg.Scopes) != 2 {
		t.Fatalf("cfg = %+v, err = %v", cfg, err)
	}
}

func TestSaveTokenRoundTripWith0600(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "token.json")
	tok := &oauth2.Token{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour).Round(time.Second)}
	if err := SaveToken(p, tok); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %v", st.Mode().Perm())
	}
	got, err := LoadToken(p)
	if err != nil || got.AccessToken != "a" || got.RefreshToken != "r" || !got.Expiry.Equal(tok.Expiry) {
		t.Fatalf("got = %+v, err = %v", got, err)
	}
}

// tokenServer fakes Google's token endpoint. Refresh requests get refreshStatus.
func tokenServer(t *testing.T, refreshStatus int) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code") != "CODE" || r.Form.Get("code_verifier") == "" {
				w.WriteHeader(400)
				w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			w.Write([]byte(`{"access_token":"new","refresh_token":"newr","token_type":"Bearer","expires_in":3600}`))
		case "refresh_token":
			w.WriteHeader(refreshStatus)
			if refreshStatus == 400 {
				w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			if refreshStatus != 200 {
				w.Write([]byte(`{"error":"server_error"}`))
				return
			}
			w.Write([]byte(`{"access_token":"refreshed","token_type":"Bearer","expires_in":3600}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testConfig(srv *httptest.Server) *oauth2.Config {
	return &oauth2.Config{
		ClientID: "cid", ClientSecret: "sec", Scopes: Scopes,
		Endpoint: oauth2.Endpoint{AuthURL: srv.URL + "/auth", TokenURL: srv.URL + "/token"},
	}
}

// browser simulates the user approving in a browser: it hits extraPaths first, then the callback.
func browser(t *testing.T, opened *int, extraPaths ...string) func(string) error {
	return func(authURL string) error {
		*opened++
		u, _ := url.Parse(authURL)
		q := u.Query()
		redirect := q.Get("redirect_uri")
		base, _ := url.Parse(redirect)
		go func() {
			for _, p := range extraPaths {
				if resp, err := http.Get(base.Scheme + "://" + base.Host + p); err == nil {
					resp.Body.Close()
				}
			}
			resp, err := http.Get(redirect + "?code=CODE&state=" + url.QueryEscape(q.Get("state")))
			if err != nil {
				t.Errorf("callback: %v", err)
				return
			}
			resp.Body.Close()
		}()
		return nil
	}
}

func TestAuthorize(t *testing.T) {
	srv := tokenServer(t, 200)
	opened := 0
	tok, err := Authorize(context.Background(), testConfig(srv), browser(t, &opened), io.Discard)
	if err != nil || tok.AccessToken != "new" || opened != 1 {
		t.Fatalf("tok = %+v, err = %v, opened = %d", tok, err, opened)
	}
}

func TestAuthorizeIgnoresOtherPaths(t *testing.T) {
	srv := tokenServer(t, 200)
	opened := 0
	tok, err := Authorize(context.Background(), testConfig(srv), browser(t, &opened, "/favicon.ico", "/"), io.Discard)
	if err != nil || tok.AccessToken != "new" {
		t.Fatalf("tok = %+v, err = %v", tok, err)
	}
}

func TestCallbackRejectsWrongState(t *testing.T) {
	codeCh, errCh := make(chan string, 1), make(chan error, 1)
	h := callbackHandler("good", codeCh, errCh)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/callback?code=X&state=bad", nil))
	if rec.Code != 400 || len(codeCh) != 0 || len(errCh) != 0 {
		t.Fatalf("code = %d, codes = %d, errs = %d", rec.Code, len(codeCh), len(errCh))
	}
}

func TestCallbackReportsDenial(t *testing.T) {
	codeCh, errCh := make(chan string, 1), make(chan error, 1)
	h := callbackHandler("s", codeCh, errCh)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/callback?error=access_denied&state=s", nil))
	if err := <-errCh; err == nil || err.Error() != "oauth: access_denied" {
		t.Fatalf("err = %v", err)
	}
}

func TestTokenSourceUsesValidStoredToken(t *testing.T) {
	srv := tokenServer(t, 200)
	p := filepath.Join(t.TempDir(), "token.json")
	SaveToken(p, &oauth2.Token{AccessToken: "stored", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)})
	opened := 0
	ts, err := TokenSource(context.Background(), testConfig(srv), p, browser(t, &opened), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := ts.Token()
	if tok.AccessToken != "stored" || opened != 0 {
		t.Fatalf("tok = %+v, opened = %d", tok, opened)
	}
}

func TestTokenSourceRefreshesAndPersists(t *testing.T) {
	srv := tokenServer(t, 200)
	p := filepath.Join(t.TempDir(), "token.json")
	SaveToken(p, &oauth2.Token{AccessToken: "old", RefreshToken: "r", Expiry: time.Now().Add(-time.Hour)})
	opened := 0
	ts, err := TokenSource(context.Background(), testConfig(srv), p, browser(t, &opened), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := ts.Token()
	saved, _ := LoadToken(p)
	if tok.AccessToken != "refreshed" || saved.AccessToken != "refreshed" || saved.RefreshToken != "r" || opened != 0 {
		t.Fatalf("tok = %+v, saved = %+v, opened = %d", tok, saved, opened)
	}
}

func TestTokenSourceReauthorizesWhenRefreshRejected(t *testing.T) {
	srv := tokenServer(t, 400)
	p := filepath.Join(t.TempDir(), "token.json")
	SaveToken(p, &oauth2.Token{AccessToken: "old", RefreshToken: "dead", Expiry: time.Now().Add(-time.Hour)})
	opened := 0
	ts, err := TokenSource(context.Background(), testConfig(srv), p, browser(t, &opened), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := ts.Token()
	saved, _ := LoadToken(p)
	if tok.AccessToken != "new" || saved.AccessToken != "new" || opened != 1 {
		t.Fatalf("tok = %+v, saved = %+v, opened = %d", tok, saved, opened)
	}
}

func TestTokenSourceReauthorizesWhenTokenFileCorrupt(t *testing.T) {
	srv := tokenServer(t, 200)
	p := filepath.Join(t.TempDir(), "token.json")
	os.WriteFile(p, []byte("{not json"), 0o600)
	opened := 0
	if _, err := TokenSource(context.Background(), testConfig(srv), p, browser(t, &opened), io.Discard); err != nil || opened != 1 {
		t.Fatalf("err = %v, opened = %d", err, opened)
	}
	var raw map[string]any
	b, _ := os.ReadFile(p)
	if json.Unmarshal(b, &raw) != nil {
		t.Fatalf("token file not rewritten: %s", b)
	}
}

func TestAuthorizeIgnoresWrongStateCallback(t *testing.T) {
	srv := tokenServer(t, 200)
	opened := 0
	tok, err := Authorize(context.Background(), testConfig(srv), browser(t, &opened, "/callback?code=X&state=bad"), io.Discard)
	if err != nil || tok.AccessToken != "new" || opened != 1 {
		t.Fatalf("tok = %+v, err = %v, opened = %d", tok, err, opened)
	}
}

func TestTokenSourceDoesNotReauthorizeOnServerError(t *testing.T) {
	srv := tokenServer(t, 503)
	p := filepath.Join(t.TempDir(), "token.json")
	SaveToken(p, &oauth2.Token{AccessToken: "old", RefreshToken: "good", Expiry: time.Now().Add(-time.Hour)})
	opened := 0
	if _, err := TokenSource(context.Background(), testConfig(srv), p, browser(t, &opened), io.Discard); err == nil || opened != 0 {
		t.Fatalf("err = %v, opened = %d", err, opened)
	}
	saved, _ := LoadToken(p)
	if saved.RefreshToken != "good" {
		t.Fatalf("refresh token discarded: %+v", saved)
	}
}

func TestTokenSourceReauthorizesWhenNoRefreshToken(t *testing.T) {
	srv := tokenServer(t, 200)
	p := filepath.Join(t.TempDir(), "token.json")
	os.WriteFile(p, []byte("{}"), 0o600)
	opened := 0
	if _, err := TokenSource(context.Background(), testConfig(srv), p, browser(t, &opened), io.Discard); err != nil || opened != 1 {
		t.Fatalf("err = %v, opened = %d", err, opened)
	}
	saved, _ := LoadToken(p)
	if saved.AccessToken != "new" {
		t.Fatalf("token file not rewritten: %+v", saved)
	}
}
