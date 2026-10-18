package aibuilder

import (
	"net/http"

	"github.com/jinzhu/gorm"

	"github.com/gazebo-web/fuel-server/auth"
	"github.com/gazebo-web/fuel-server/bundles/users"
)

// Authenticate authenticates requests the way fuel-server's routes do: a
// JWT the verifier accepts (or a Private-Token), then the shared user
// resolver.
func Authenticate(db *gorm.DB, v *auth.Verifier) func(http.Handler) http.Handler {
	check := v.Handler(func() *gorm.DB { return db })
	return func(next http.Handler) http.Handler {
		return check(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, em := users.FromRequest(db, r)
			if em != nil {
				writeError(w, em.StatusCode, em.Msg)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
		}))
	}
}
