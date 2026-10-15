// Package auth handles the OAuth desktop flow and token persistence.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var Scopes = []string{
	"https://www.googleapis.com/auth/spreadsheets.readonly",
	"https://www.googleapis.com/auth/drive.metadata.readonly",
}

var ErrNoCredentials = errors.New("credentials.json が見つかりません")

// LoadOAuthConfig reads an OAuth client JSON downloaded from Google Cloud Console.
func LoadOAuthConfig(credPath string) (*oauth2.Config, error) {
	b, err := os.ReadFile(credPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNoCredentials, credPath)
	}
	if err != nil {
		return nil, err
	}
	return google.ConfigFromJSON(b, Scopes...)
}

// storedToken is the token.json format: the token plus the OAuth client that issued it.
type storedToken struct {
	oauth2.Token
	ClientID string `json:"client_id,omitempty"`
}

// LoadToken reads token.json and returns the token and the client ID it was issued to
// ("" for files written before the client ID was recorded).
func LoadToken(path string) (*oauth2.Token, string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var st storedToken
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, "", err
	}
	return &st.Token, st.ClientID, nil
}

// SaveToken writes tok and the issuing clientID atomically with mode 0600, creating the directory with 0700.
func SaveToken(path, clientID string, tok *oauth2.Token) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".token-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if err := json.NewEncoder(f).Encode(storedToken{Token: *tok, ClientID: clientID}); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Authorize runs the loopback flow: it serves /callback on 127.0.0.1, opens the
// consent page and exchanges the returned code (with PKCE) for a token.
func Authorize(ctx context.Context, cfg *oauth2.Config, openURL func(string) error, out io.Writer) (*oauth2.Token, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	c := *cfg
	c.RedirectURL = "http://" + ln.Addr().String() + "/callback"
	state, err := randomState()
	if err != nil {
		ln.Close()
		return nil, err
	}
	verifier := oauth2.GenerateVerifier()
	authURL := c.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("prompt", "consent"))

	codeCh, errCh := make(chan string, 1), make(chan error, 1)
	mux := http.NewServeMux()
	mux.Handle("/callback", callbackHandler(state, codeCh, errCh))
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	defer srv.Close()

	fmt.Fprintf(out, "ブラウザで Google アカウントへのアクセスを許可してください:\n%s\n", authURL)
	if openURL != nil {
		_ = openURL(authURL)
	}
	select {
	case code := <-codeCh:
		return c.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func callbackHandler(state string, codeCh chan<- string, errCh chan<- error) http.Handler {
	fail := func(w http.ResponseWriter, err error) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		select {
		case errCh <- err:
		default:
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			// A stray or forged request must not abort the flow; keep waiting for the real callback.
			http.Error(w, "oauth: state mismatch", http.StatusBadRequest)
			return
		}
		if e := q.Get("error"); e != "" {
			fail(w, errors.New("oauth: "+e))
			return
		}
		code := q.Get("code")
		if code == "" {
			fail(w, errors.New("oauth: missing code"))
			return
		}
		fmt.Fprintln(w, "gsv: 認可が完了しました。ターミナルに戻ってください。")
		select {
		case codeCh <- code:
		default:
		}
	})
}

func randomState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// TokenSource returns a token source backed by tokenPath. It refreshes the stored
// token up front and falls back to the browser flow when the token is missing,
// unreadable, issued to a different OAuth client, or rejected by Google.
// Refreshed tokens are written back to tokenPath.
func TokenSource(ctx context.Context, cfg *oauth2.Config, tokenPath string, openURL func(string) error, out io.Writer) (oauth2.TokenSource, error) {
	tok, clientID, err := LoadToken(tokenPath)
	if err == nil && clientID != cfg.ClientID {
		fmt.Fprintln(out, "OAuth クライアント (credentials.json) が変わりました。再認可します。")
	} else if err == nil && !tok.Valid() && tok.RefreshToken == "" {
		fmt.Fprintln(out, "保存済みのトークンを更新できません。再認可します。")
	} else if err == nil {
		ts := cfg.TokenSource(ctx, tok)
		fresh, err := ts.Token()
		if err == nil {
			if fresh.AccessToken != tok.AccessToken {
				if err := SaveToken(tokenPath, cfg.ClientID, fresh); err != nil {
					return nil, err
				}
			}
			return &persistingSource{base: ts, path: tokenPath, clientID: cfg.ClientID, last: fresh.AccessToken}, nil
		}
		var re *oauth2.RetrieveError
		if !errors.As(err, &re) || re.ErrorCode != "invalid_grant" {
			return nil, err
		}
		fmt.Fprintln(out, "保存済みのトークンが無効になりました。再認可します。")
	} else if !errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(out, "トークンを読み込めませんでした (%v)。再認可します。\n", err)
	}
	tok, err = Authorize(ctx, cfg, openURL, out)
	if err != nil {
		return nil, err
	}
	if err := SaveToken(tokenPath, cfg.ClientID, tok); err != nil {
		return nil, err
	}
	return &persistingSource{base: cfg.TokenSource(ctx, tok), path: tokenPath, clientID: cfg.ClientID, last: tok.AccessToken}, nil
}

type persistingSource struct {
	base     oauth2.TokenSource
	path     string
	clientID string
	mu       sync.Mutex
	last     string
}

func (s *persistingSource) Token() (*oauth2.Token, error) {
	tok, err := s.base.Token()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if tok.AccessToken != s.last {
		s.last = tok.AccessToken
		_ = SaveToken(s.path, s.clientID, tok) // a failed write only costs a refresh next launch
	}
	return tok, nil
}
