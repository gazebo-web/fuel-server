package users

import (
	"net/http"

	"github.com/gazebo-web/gz-go/v7"
	"github.com/jinzhu/gorm"

	"github.com/gazebo-web/fuel-server/auth"
)

// FromRequest returns the user a request is authenticated as: the owner of
// its Private-Token (a personal access token), which supersedes a JWT, or
// the user whose identity is the subject of its JWT. The JWT must already
// be validated by the auth package's middleware. It is the one resolver for the web
// server's routes and the AI Builder agent.
func FromRequest(tx *gorm.DB, r *http.Request) (*User, *gz.ErrMsg) {
	if token := r.Header.Get("Private-Token"); len(token) > 0 {
		accessToken, err := gz.ValidateAccessToken(token, tx)
		if err != nil {
			return nil, gz.NewErrorMessage(gz.ErrorUnauthorized)
		}
		user := new(User)
		if err := tx.Where("id = ?", accessToken.UserID).First(user).Error; err != nil {
			return nil, gz.NewErrorMessage(gz.ErrorUnauthorized)
		}
		return user, nil
	}
	identity, valid := auth.Identity(r)
	if !valid {
		return nil, gz.NewErrorMessage(gz.ErrorAuthJWTInvalid)
	}
	return ByIdentity(tx, identity, false)
}
