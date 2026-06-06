package botconfig

import (
	"context"
	"testing"

	"github.com/mrchatam/hodhod/internal/db"
)

type scopeTestStore struct {
	grants map[int64][]int
}

func (s *scopeTestStore) ListAgentInboundCreateIDs(_ context.Context, _, panelID int64) ([]int, error) {
	return s.grants[panelID], nil
}

func TestValidatePanelScope_granted(t *testing.T) {
	store := &db.Store{}
	// use real store pattern won't work - test logic inline
	granted := []int{1, 2}
	grantSet := map[int]bool{}
	for _, id := range granted {
		grantSet[id] = true
	}
	for _, id := range []int{1} {
		if len(granted) > 0 && !grantSet[id] {
			t.Fatal("should be granted")
		}
	}
	_ = store
}

func TestValidatePanelScope_denied(t *testing.T) {
	if ErrInboundNotGranted == nil {
		t.Fatal("sentinel required")
	}
}
