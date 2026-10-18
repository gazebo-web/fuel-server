package auth

import (
	"log"
	"net/http"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/jinzhu/gorm"
)

// The middlewares gz-go's router runs before secure and optional routes
// (gz.RouterConfigurer.SetAuthHandlers), with the old middleware's
// behaviour: 401 and a plain-text reason for a bad token, OPTIONS requests
// let through, and a Private-Token (personal access token) accepted in
// place of a JWT on secure routes.

// Optional passes requests without a token and checks the ones with one.
func (v *Verifier) Optional() negroni.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		v.check(w, r, next, true)
	}
}

// Required needs a valid JWT, or a valid Private-Token, whose last use it
// records in db's access tokens.
func (v *Verifier) Required(db func() *gorm.DB) negroni.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		if token := r.Header.Get("Private-Token"); token != "" {
			var tx *gorm.DB
			if db != nil {
				tx = db()
			}
			if err := usePrivateToken(tx, token); err != "" {
				http.Error(w, err, http.StatusUnauthorized)
				return
			}
			next(w, r)
			return
		}
		v.check(w, r, next, false)
	}
}

// Handler is Required as net/http middleware, for servers that don't use
// gz-go's router (the AI Builder agent).
func (v *Verifier) Handler(db func() *gorm.DB) func(http.Handler) http.Handler {
	req := v.Required(db)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			req(w, r, next.ServeHTTP)
		})
	}
}

func (v *Verifier) check(w http.ResponseWriter, r *http.Request, next http.HandlerFunc, optional bool) {
	if r.Method == http.MethodOptions {
		next(w, r)
		return
	}
	token, err := bearer(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if token == "" {
		if optional {
			next(w, r)
			return
		}
		http.Error(w, "Required authorization token not found", http.StatusUnauthorized)
		return
	}
	claims, err := v.Verify(r.Context(), token)
	if err != nil {
		http.Error(w, "Error parsing token: "+err.Error(), http.StatusUnauthorized)
		return
	}
	next(w, r.WithContext(WithClaims(r.Context(), claims)))
}

// usePrivateToken validates a personal access token and records its use,
// returning the reason it was refused, if it was.
func usePrivateToken(db *gorm.DB, token string) string {
	if db == nil {
		return "no database"
	}
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error.Error()
	}
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()
	at, em := gz.ValidateAccessToken(token, tx)
	if em != nil {
		tx.Rollback()
		log.Printf("auth: access token: %s", em.Msg)
		return em.Msg
	}
	now := time.Now()
	at.LastUsed = &now
	tx.Save(at)
	if err := tx.Commit().Error; err != nil {
		return err.Error()
	}
	return ""
}
