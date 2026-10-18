package aibuilder

import (
	"net/http"

	"github.com/gazebo-web/gz-go/v7"
	"github.com/jinzhu/gorm"

	"github.com/gazebo-web/fuel-server/bundles/users"
)

// GzAuth authenticates requests the way fuel-server's routes do: gz's JWT
// middleware (or a Private-Token), then the shared user resolver.
func GzAuth(db *gorm.DB, server *gz.Server) func(http.Handler) http.Handler {
	jwt := gz.CreateJWTRequiredMiddleware(server)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			jwt(w, r, func(w http.ResponseWriter, r *http.Request) {
				u, em := users.FromRequest(db, r)
				if em != nil {
					writeError(w, em.StatusCode, em.Msg)
					return
				}
				next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
			})
		})
	}
}
