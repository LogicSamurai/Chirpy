package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// type CustomClaim struct {
// 	UserID uuid.UUID `json:"sub"`
// 	jwt.RegisteredClaims
// }

func HashPassword(password string) (string, error) {
	hash, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		log.Fatal(err)
	}

	return hash, nil
}

func CheckPasswordHash(password, hash string) (bool, error) {
	is_correct, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		log.Fatal(err)
	}
	return is_correct, nil
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
    now := time.Now().UTC()
    expiryTime := now.Add(expiresIn)
    
    fmt.Printf("Current UTC time: %v\n", now)
    fmt.Printf("Token expires at: %v\n", expiryTime)
    fmt.Printf("Expires in: %v\n", expiresIn)
    
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
        Issuer:    "chirpy-access",
        IssuedAt:  jwt.NewNumericDate(now),
        ExpiresAt: jwt.NewNumericDate(expiryTime),
        Subject:   userID.String(),
    })

    signedToken, err := token.SignedString([]byte(tokenSecret))
    if err != nil {
        return "", err
    }

    fmt.Println("Signed Token============", signedToken)
    
    return signedToken, nil
}


func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(tokenSecret), nil
	})

	if err != nil {
		return uuid.Nil, err
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return uuid.Nil, jwt.ErrSignatureInvalid
	}

	userId, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, err
	}

	return userId, nil
}

func GetBearerToken(headers http.Header) (string, error) {
	if len(headers) == 0 {
		return "", errors.New("Missing Headers")
	}

	bearerToken := headers.Get("Authorization")

	token := strings.TrimPrefix(bearerToken, "Bearer ")
	token = strings.TrimSpace(token)

	return token, nil
}

func MakeRefreshToken() string {
	randomString := make([]byte, 32)
	rand.Read(randomString)

	return hex.EncodeToString(randomString)
}

func GetAPIKey(headers http.Header) (string, error) {
	authHeader := headers.Get("Authorization")
	if authHeader == "" {
		return "", errors.New("Missing Authorization headers")
	}

	parts := strings.Fields(authHeader)
	if len(parts) == 2 && parts[0] == "ApiKey" {
		return parts[1], nil
	}

	return "", errors.New("Missing ApiKey headers")
}
