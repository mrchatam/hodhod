package panels

import (
	"context"
	"time"
)

// OnlineTracker reports live client connectivity (3x-ui).
type OnlineTracker interface {
	ListOnlineUsernames(ctx context.Context) (map[string]bool, error)
	ListLastOnline(ctx context.Context) (map[string]time.Time, error)
}

// ServerStatusInfo is a normalized panel health snapshot.
type ServerStatusInfo struct {
	CPUPct     float64
	MemUsed    int64
	MemTotal   int64
	DiskUsed   int64
	DiskTotal  int64
	NetUp      int64
	NetDown    int64
	XrayState  string
	XrayVer    string
	TCPCount   int
	Load1      float64
	Online     int
	Reachable  bool
	Err        string
}

// ServerMonitor fetches machine health from a panel.
type ServerMonitor interface {
	ServerStatus(ctx context.Context) (*ServerStatusInfo, error)
}

// DepletedCleaner removes expired/depleted clients (3x-ui).
type DepletedCleaner interface {
	DeleteDepletedClients(ctx context.Context) (int, error)
}
