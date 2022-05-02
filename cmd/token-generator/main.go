package main

// TO build this token-generator command (from go_ws/src):
// $ go install github.com/gazebo-web/fuel-server/cmd/token-generator

// Import this file's dependencies
import (
	"errors"
	"github.com/dgrijalva/jwt-go"
	"github.com/gazebo-web/fuel-server/cmd/token-generator/generator"
	"log"
	"os"
)

var gTestPrivateKey string

// Update this value with the identity you want to use.
// The generated token will have that identity.
// See init() func
var gIdentity string

// Read an environment variable and return an error if not present
func readEnvVar(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", errors.New("Missing " + name + " env variable.")
	}
	return value, nil
}

func init() {
	gIdentity = "test-user-identity"

	var err error
	// RSA256 private key WITHOUT the -----BEGIN RSA PRIVATE KEY----- and -----END RSA PRIVATE KEY-----
	if gTestPrivateKey, err = readEnvVar("TOKEN_GENERATOR_PRIVATE_RSA256_KEY"); err != nil {
		log.Printf("Missing TOKEN_GENERATOR_PRIVATE_RSA256_KEY env variable." +
			"Won't be able to generate jwt token.")
	}
}

func main() {
	jwtClaims := jwt.MapClaims{
		"sub": gIdentity,
	}

	testPrivateKeyAsPEM := []byte("-----BEGIN RSA PRIVATE KEY-----\n" + gTestPrivateKey + "\n-----END RSA PRIVATE KEY-----")
	ss, err := generator.GenerateTokenRSA256(testPrivateKeyAsPEM, jwtClaims)
	if err != nil {
		log.Println("token-generator: error while generating token", err)
	}
	log.Println("Signed token: ", ss)
}
