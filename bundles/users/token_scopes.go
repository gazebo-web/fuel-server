package users

import (
	"github.com/gazebo-web/gz-go/v7"
	"github.com/jinzhu/gorm"
)

// AccessTokenScope is one scope granted to a personal access token
// (design §4.5): gz-go's AccessToken has no scope field. A token with no
// rows predates scopes and keeps full access.
type AccessTokenScope struct {
	ID      uint   `gorm:"primary_key"`
	TokenID uint   `gorm:"index"`
	Scope   string `gorm:"type:varchar(64)"`
}

// TableName names the table after the design.
func (AccessTokenScope) TableName() string { return "access_token_scopes" }

// TokenScopes returns the scopes granted to a token, or nil for full
// access.
func TokenScopes(tx *gorm.DB, tokenID uint) ([]string, error) {
	var rows []AccessTokenScope
	if err := tx.Where("token_id = ?", tokenID).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Scope
	}
	return out, nil
}

// setTokenScopes records the scopes of a new token, refusing unknown ones.
func setTokenScopes(tx *gorm.DB, tokenID uint, scopes, allowed []string) *gz.ErrMsg {
	ok := map[string]bool{}
	for _, s := range allowed {
		ok[s] = true
	}
	for _, s := range scopes {
		if !ok[s] {
			em := gz.NewErrorMessage(gz.ErrorFormInvalidValue)
			em.Msg = "unknown scope " + s
			return em
		}
		if err := tx.Create(&AccessTokenScope{TokenID: tokenID, Scope: s}).Error; err != nil {
			return gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
		}
	}
	return nil
}
