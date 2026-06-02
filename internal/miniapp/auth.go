package miniapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ValidateInitData verifies Telegram WebApp initData per official algorithm.
func ValidateInitData(initData, botToken string, maxAge time.Duration) (telegramUserID int64, err error) {
	vals, err := url.ParseQuery(initData)
	if err != nil {
		return 0, err
	}
	hashHex := vals.Get("hash")
	if hashHex == "" {
		return 0, fmt.Errorf("missing hash")
	}
	vals.Del("hash")
	var pairs []string
	for k := range vals {
		pairs = append(pairs, k+"="+vals.Get(k))
	}
	sort.Strings(pairs)
	dataCheck := strings.Join(pairs, "\n")
	secretKey := hmac.New(sha256.New, []byte("WebAppData"))
	secretKey.Write([]byte(botToken))
	mac := hmac.New(sha256.New, secretKey.Sum(nil))
	mac.Write([]byte(dataCheck))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(hashHex)) {
		return 0, fmt.Errorf("invalid initData hash")
	}
	if authDate := vals.Get("auth_date"); authDate != "" {
		ts, _ := strconv.ParseInt(authDate, 10, 64)
		if time.Since(time.Unix(ts, 0)) > maxAge {
			return 0, fmt.Errorf("initData expired")
		}
	}
	userJSON := vals.Get("user")
	if userJSON == "" {
		return 0, fmt.Errorf("missing user")
	}
	const needle = `"id":`
	idx := strings.Index(userJSON, needle)
	if idx < 0 {
		return 0, fmt.Errorf("invalid user")
	}
	var id int64
	fmt.Sscanf(userJSON[idx+len(needle):], "%d", &id)
	return id, nil
}
