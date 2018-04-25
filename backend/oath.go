package backend

import (
	"github.com/jinzhu/gorm"
	"github.com/nlopes/slack"
)

// TokenDataInterface describes the behavior of accessing token data
type TokenDataInterface interface {
	CreateTokenData(t *TokenData) error
	GetTokenDataByUserID(id string) (*TokenData, error)
}

// TokenData stores the OAuthResponse details from users
type TokenData struct {
	gorm.Model
	slack.OAuthResponse
}

// CreateTokenData adds token data to the database
func (b *Backend) CreateTokenData(t *TokenData) error {
	if result := b.db.Create(t); result.Error != nil {
		return ErrDatabaseGeneral(result.Error.Error())
	}
	return nil
}

// GetTokenDataByUserID gets token data by the UserID
func (b *Backend) GetTokenDataByUserID(id string) (TokenData, error) {
	var t TokenData
	oauthFilter := slack.OAuthResponse{
		UserID: id,
	}
	filter := TokenData{
		OAuthResponse: oauthFilter,
	}
	if result := b.db.Where(&filter).First(&t); result.Error != nil {
		return t, ErrDatabaseGeneral(result.Error.Error())
	}
	return t, nil
}
