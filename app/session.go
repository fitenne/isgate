package app

import (
	"database/sql"
	"encoding/gob"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"

	_ "github.com/mattn/go-sqlite3"
)

const sessionSchema = `
PRAGMA journal_mode=WAL;
CREATE TABLE sessions (
	token TEXT PRIMARY KEY,
	data BLOB NOT NULL,
	expiry REAL NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions(expiry);
`

type Tokens struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
}

func init() {
	gob.Register(&Tokens{})
}

func NewSessionManager(c *Config) (*scs.SessionManager, error) {
	var (
		sessionDBPath   = path.Join(c.Session.DataDir, "session.db")
		isEmptyDatabase = false
	)

	if _, err := os.Stat(sessionDBPath); errors.Is(err, os.ErrNotExist) {
		isEmptyDatabase = true
	}

	db, err := sql.Open("sqlite3", path.Join(c.Session.DataDir, "session.db"))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if isEmptyDatabase {
		_, err := db.Exec(sessionSchema)
		if err != nil {
			return nil, fmt.Errorf("failed to create schema: %w", err)
		}
	}

	sessionManager := scs.New()
	sessionManager.Lifetime = 30 * 24 * time.Hour
	sessionManager.IdleTimeout = 48 * time.Hour
	sessionManager.Store = sqlite3store.New(db)
	sessionManager.Cookie = scs.SessionCookie{
		Name:     "__Http-session",
		Domain:   c.Session.CookieDomain,
		HttpOnly: true,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
		Persist:  true,
	}

	return sessionManager, nil
}
