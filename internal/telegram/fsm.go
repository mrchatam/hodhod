package telegram

import (
	"fmt"
	"sync"
)

type userState struct {
	Step   string
	Amount int64
}

type stateStore struct {
	mu sync.Map
}

func (s *stateStore) get(key string) userState {
	v, ok := s.mu.Load(key)
	if !ok {
		return userState{}
	}
	return v.(userState)
}

func (s *stateStore) set(key string, st userState) {
	s.mu.Store(key, st)
}

func (s *stateStore) clear(key string) {
	s.mu.Delete(key)
}

func stateKey(botID, userID int64) string {
	return fmt.Sprintf("%d:%d", botID, userID)
}
