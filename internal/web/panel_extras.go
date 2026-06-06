package web

import (
	"context"

	"github.com/mrchatam/hodhod/internal/panels"
)

func enrichPanelUserOnline(ctx context.Context, client panels.Client, rows []PanelUserRow) []PanelUserRow {
	tracker, ok := client.(panels.OnlineTracker)
	if !ok || len(rows) == 0 {
		return rows
	}
	online, _ := tracker.ListOnlineUsernames(ctx)
	lastOnline, _ := tracker.ListLastOnline(ctx)
	for i := range rows {
		if online != nil && online[rows[i].Username] {
			rows[i].Online = true
		}
		if lastOnline != nil {
			if t, ok := lastOnline[rows[i].Username]; ok {
				rows[i].LastOnline = t
			}
		}
	}
	return rows
}

type panelHealthRow struct {
	PanelID   int64
	Name      string
	Reachable bool
	CPUPct    float64
	MemPct    float64
	XrayState string
	Online    int
	Err       string
}

func (s *Server) panelHealthRows(ctx context.Context) []panelHealthRow {
	panelsList, err := s.Store.ListPanels(ctx)
	if err != nil {
		return nil
	}
	var rows []panelHealthRow
	for _, p := range panelsList {
		if p.Status != "active" {
			continue
		}
		row := panelHealthRow{PanelID: p.ID, Name: p.Name}
		client, err := s.Panels.Get(ctx, p.ID)
		if err != nil {
			row.Err = err.Error()
			rows = append(rows, row)
			continue
		}
		mon, ok := client.(panels.ServerMonitor)
		if !ok {
			row.Reachable = true
			row.XrayState = "n/a"
			rows = append(rows, row)
			continue
		}
		st, err := mon.ServerStatus(ctx)
		if err != nil || st == nil {
			if st != nil {
				row.Err = st.Err
			} else if err != nil {
				row.Err = err.Error()
			}
			rows = append(rows, row)
			continue
		}
		row.Reachable = st.Reachable
		row.CPUPct = st.CPUPct
		if st.MemTotal > 0 {
			row.MemPct = float64(st.MemUsed) / float64(st.MemTotal) * 100
		}
		row.XrayState = st.XrayState
		row.Online = st.Online
		row.Err = st.Err
		rows = append(rows, row)
	}
	return rows
}
