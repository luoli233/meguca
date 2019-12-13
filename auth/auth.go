// Package auth determines and asserts client permissions to access and modify
// server resources.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/bakape/meguca/common"
	"github.com/bakape/meguca/config"
	"github.com/jackc/pgx/pgtype"
)

// GetIP extracts the IP of a request, honouring reverse proxies, if set
func GetIP(r *http.Request) (ip net.IP, err error) {
	var s string

	if config.Server.Server.ReverseProxied {
		h := r.Header.Get("X-Forwarded-For")
		if h != "" {
			if i := strings.LastIndexByte(h, ','); i != -1 {
				h = h[i+1:]
			}

			s = strings.TrimSpace(h) // Header can contain padding spaces
		}
	}
	if s == "" {
		s = r.RemoteAddr
	}

	split, _, err := net.SplitHostPort(s)
	if err == nil {
		// Port in address
		s = split
	} else {
		err = nil
	}

	ip = net.ParseIP(s)
	if ip == nil {
		err = fmt.Errorf("invalid IP: %s", s)
	}
	return
}

// RandomID generates a randomID of base64 characters of desired byte length
func RandomID(length int) (string, error) {
	buf := make([]byte, length)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(buf), nil
}

// 64 byte token that JSON/text en/decodes to a raw URL-safe encoding base64
// string
type Token [64]byte

func (t Token) MarshalText() ([]byte, error) {
	buf := make([]byte, 86)
	base64.RawURLEncoding.Encode(buf[:], t[:])
	return buf, nil
}

func (t *Token) UnmarshalText(buf []byte) error {
	if len(buf) != 86 {
		return ErrInvalidToken
	}

	n, err := base64.RawURLEncoding.Decode(t[:], buf)
	if n != 64 || err != nil {
		return ErrInvalidToken
	}
	return nil
}

// Implement pgtype.Encoder
func (t Token) EncodeBinary(ci *pgtype.ConnInfo, buf []byte) (
	[]byte, error,
) {
	return append(buf, t[:]...), nil
}

// Create new Token populated by cryptographically secure random data
func NewAuthToken() (t Token, err error) {
	n, err := rand.Read(t[:])
	if err == nil && n != 64 {
		err = fmt.Errorf("auth: not enough data read: %d", n)
	}
	return
}

// Extract user auth token from request
func ExtractToken(r *http.Request) (user Token, err error) {
	err = user.UnmarshalText(
		[]byte(
			strings.TrimPrefix(
				r.Header.Get("Authorization"),
				"Bearer ",
			),
		),
	)
	if err != nil {
		err = common.StatusError{
			Err:  errors.New("invalid authentication key"),
			Code: 403,
		}
	}
	return
}
