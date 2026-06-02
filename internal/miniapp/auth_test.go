package miniapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestValidateInitData_valid(t *testing.T) {
	token := "123456:ABC-DEF"
	vals := url.Values{}
	vals.Set("auth_date", strconv.FormatInt(time.Now().Unix(), 10))
	vals.Set("user", `{"id":42,"first_name":"Test"}`)
	var pairs []string
	for k := range vals {
		pairs = append(pairs, k+"="+vals.Get(k))
	}
	sort.Strings(pairs)
	dataCheck := strings.Join(pairs, "\n")
	secretKey := hmac.New(sha256.New, []byte("WebAppData"))
	secretKey.Write([]byte(token))
	key := secretKey.Sum(nil)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(dataCheck))
	vals.Set("hash", hex.EncodeToString(mac.Sum(nil)))
	id, err := ValidateInitData(vals.Encode(), token, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if id != 42 {
		t.Fatalf("got %d", id)
	}
}

func TestValidateInitData_tampered(t *testing.T) {
	_, err := ValidateInitData("hash=bad&user={}", "token", time.Hour)
	if err == nil {
		t.Fatal("expected error")
	}
}
