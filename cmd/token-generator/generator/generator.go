package generator

// TO build this token-generator command (from go_ws/src):
// $ go install gitlab.com/ignitionrobotics/web/fuelserver/cmd/token-generator

// Import this file's dependencies
import (
	"encoding/base64"
	"github.com/dgrijalva/jwt-go"
	"log"
)

// GenerateTokenHS256 generates an HS256 token containing the given claims,
// the returns the token signed with the given client secret.
// Used with single keys (ie. client secret)
func GenerateTokenHS256(base64ClientSecret string, jwtClaims jwt.MapClaims) (signedToken string, err error) {

	signingKey, err := base64.URLEncoding.DecodeString(base64ClientSecret)
	if err != nil {
		log.Println("error while decoding client secret", err)
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims)
	ss, err := token.SignedString(signingKey)
	if err != nil {
		log.Println("error while encoding token into signed string", err)
		return
	}
	// All OK !
	signedToken = ss
	return
}

// GenerateTokenRSA256 generates an RSA256 token containing the given claims,
// the returns the token signed with the given PEM private key.
// Used with public - private keys.
func GenerateTokenRSA256(pemPrivKey []byte, jwtClaims jwt.MapClaims) (signedToken string, err error) {
	signingKey, err := jwt.ParseRSAPrivateKeyFromPEM(pemPrivKey)
	if err != nil {
		log.Println("error while parsing private key", err)
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwtClaims)
	ss, err := token.SignedString(signingKey)
	if err != nil {
		log.Println("error while encoding token into signed string", err)
		return
	}
	// All OK !
	signedToken = ss
	return
}
