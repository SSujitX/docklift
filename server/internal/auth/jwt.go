package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Token purposes. Purpose tokens must never authenticate normal API calls.
const (
	PurposeSSE      = "sse"
	PurposeTerminal = "terminal"
)

// Token lifetimes, as passed to jsonwebtoken's expiresIn.
const (
	SessionTTL = 7 * 24 * time.Hour // '7d'
	PurposeTTL = 5 * time.Minute    // '5m'
)

// Verification errors, mirroring jsonwebtoken's error classes.
var (
	// ErrInvalidToken is JsonWebTokenError (malformed, bad signature, bad algorithm, bad claims).
	ErrInvalidToken = errors.New("invalid token")
	// ErrTokenExpired is TokenExpiredError.
	ErrTokenExpired = errors.New("token expired")
	// ErrTokenNotActive is NotBeforeError, which the TypeScript middleware
	// answered with 500 "Authentication failed".
	ErrTokenNotActive = errors.New("token not active")
)

// signedClaims is the payload jsonwebtoken produced, in the same key order:
// {userId, email, role, purpose?, pwdv, iat, exp}.
type signedClaims struct {
	UserID  string `json:"userId"`
	Email   string `json:"email"`
	Role    string `json:"role"`
	Purpose string `json:"purpose,omitempty"`
	Pwdv    int64  `json:"pwdv"`
	Iat     int64  `json:"iat"`
	Exp     int64  `json:"exp"`
}

func (c signedClaims) GetExpirationTime() (*jwt.NumericDate, error) {
	return jwt.NewNumericDate(time.Unix(c.Exp, 0)), nil
}
func (c signedClaims) GetIssuedAt() (*jwt.NumericDate, error) {
	return jwt.NewNumericDate(time.Unix(c.Iat, 0)), nil
}
func (c signedClaims) GetNotBefore() (*jwt.NumericDate, error) { return nil, nil }
func (c signedClaims) GetIssuer() (string, error)              { return "", nil }
func (c signedClaims) GetSubject() (string, error)             { return "", nil }
func (c signedClaims) GetAudience() (jwt.ClaimStrings, error)  { return nil, nil }

// TokenSubject is who a token is issued for.
type TokenSubject struct {
	UserID string
	Email  string
	Role   string
	// PasswordVersion is the pwdv claim (passwordChangedAt in Unix ms, or 0).
	PasswordVersion int64
}

// sign issues an HS256 token valid for ttl from now.
func sign(secret string, sub TokenSubject, purpose string, ttl time.Duration, now time.Time) (string, error) {
	iat := now.Unix()
	claims := signedClaims{
		UserID: sub.UserID, Email: sub.Email, Role: sub.Role, Purpose: purpose,
		Pwdv: sub.PasswordVersion, Iat: iat, Exp: iat + int64(ttl/time.Second),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return token, nil
}

// Claims is a verified token payload with JavaScript-faithful typing: fields
// hold their value only when the claim was a string (or number for pwdv).
type Claims struct {
	// UserID is the userId claim when it is a string.
	UserID string
	// userIDState records how userId must be treated by the password check.
	userIDState userIDState
	Email       string
	Role        string
	Purpose     string
	// Pwdv is the pwdv claim when it is a number.
	Pwdv    float64
	HasPwdv bool
}

type userIDState int

const (
	userIDFalsy   userIDState = iota // absent, null, "", 0 or false: no password check
	userIDString                     // a non-empty string
	userIDInvalid                    // another truthy type (a database lookup would fail)
)

// parser accepts HS256 only: every token Docklift issues (TS or Go) is HS256,
// so other HMAC variants are refused rather than verified with the same key.
var parser = jwt.NewParser(
	jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	jwt.WithoutClaimsValidation(),
	jwt.WithJSONNumber(),
)

// verify checks a token like jsonwebtoken.verify(token, secret) restricted
// to HS256: signature first, then `nbf` (NotBeforeError) and `exp`
// (TokenExpiredError when now >= exp, whole seconds).
func verify(secret, token string, now time.Time) (*Claims, error) {
	parsed, err := parser.Parse(token, func(*jwt.Token) (any, error) { return []byte(secret), nil })
	if err != nil || !parsed.Valid {
		return nil, ErrInvalidToken
	}
	mc, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}
	clock := float64(now.Unix())
	if v, present := mc["nbf"]; present {
		nbf, ok := number(v)
		if !ok {
			return nil, ErrInvalidToken
		}
		if nbf > clock {
			return nil, ErrTokenNotActive
		}
	}
	if v, present := mc["exp"]; present {
		exp, ok := number(v)
		if !ok {
			return nil, ErrInvalidToken
		}
		if clock >= exp {
			return nil, ErrTokenExpired
		}
	}

	c := &Claims{}
	switch v := mc["userId"].(type) {
	case string:
		if v != "" {
			c.UserID, c.userIDState = v, userIDString
		}
	case nil, bool:
		if b, isBool := v.(bool); isBool && b {
			c.userIDState = userIDInvalid
		}
	case json.Number:
		if f, ok := number(v); ok && f != 0 && !math.IsNaN(f) {
			c.userIDState = userIDInvalid
		}
	default:
		c.userIDState = userIDInvalid
	}
	c.Email, _ = mc["email"].(string)
	c.Role, _ = mc["role"].(string)
	c.Purpose, _ = mc["purpose"].(string)
	if v, present := mc["pwdv"]; present {
		c.Pwdv, c.HasPwdv = number(v)
	}
	return c, nil
}

func number(v any) (float64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := n.Float64()
	return f, err == nil
}
