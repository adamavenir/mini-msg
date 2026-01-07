package chat

import (
	"database/sql"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/lipgloss"
)

var agentPalette = []lipgloss.Color{
	lipgloss.Color("111"),
	lipgloss.Color("157"),
	lipgloss.Color("216"),
	lipgloss.Color("36"),
	lipgloss.Color("183"),
	lipgloss.Color("230"),
}

func buildColorMap(dbConn *sql.DB, lookback int, includeArchived bool) (map[string]lipgloss.Color, error) {
	messages, err := db.GetMessages(dbConn, &types.MessageQueryOptions{Limit: lookback, IncludeArchived: includeArchived})
	if err != nil {
		return nil, err
	}

	lastSeen := map[string]int64{}
	for _, msg := range messages {
		if msg.Type != types.MessageTypeAgent {
			continue
		}
		parsed, err := core.ParseAgentID(msg.FromAgent)
		if err != nil {
			continue
		}
		if ts, ok := lastSeen[parsed.Base]; !ok || msg.TS > ts {
			lastSeen[parsed.Base] = msg.TS
		}
	}

	ordered := make([]string, 0, len(lastSeen))
	for base := range lastSeen {
		ordered = append(ordered, base)
	}
	sortByLastSeen(ordered, lastSeen)

	colorMap := make(map[string]lipgloss.Color)
	for idx, base := range ordered {
		colorMap[base] = agentPalette[idx%len(agentPalette)]
	}
	return colorMap, nil
}

func sortByLastSeen(bases []string, lastSeen map[string]int64) {
	for i := 0; i < len(bases); i++ {
		for j := i + 1; j < len(bases); j++ {
			if lastSeen[bases[j]] > lastSeen[bases[i]] {
				bases[i], bases[j] = bases[j], bases[i]
			}
		}
	}
}

func colorForAgent(agentID string, colorMap map[string]lipgloss.Color) lipgloss.Color {
	parsed, err := core.ParseAgentID(agentID)
	base := agentID
	if err == nil {
		base = parsed.Base
	}
	if color, ok := colorMap[base]; ok {
		return color
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(base))
	idx := int(h.Sum32()) % len(agentPalette)
	return agentPalette[idx]
}

func contrastTextColor(color lipgloss.Color) lipgloss.Color {
	code, ok := parseColorCode(color)
	if !ok {
		return lipgloss.Color("231")
	}
	r, g, b := colorCodeToRGB(code)
	luminance := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
	if luminance > 128 {
		return lipgloss.Color("16")
	}
	return lipgloss.Color("231")
}

func parseColorCode(color lipgloss.Color) (int, bool) {
	trimmed := strings.TrimSpace(string(color))
	if trimmed == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func colorCodeToRGB(code int) (int, int, int) {
	if code < 16 {
		standard := [16][3]int{
			{0, 0, 0}, {128, 0, 0}, {0, 128, 0}, {128, 128, 0},
			{0, 0, 128}, {128, 0, 128}, {0, 128, 128}, {192, 192, 192},
			{128, 128, 128}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0},
			{0, 0, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
		}
		values := standard[code]
		return values[0], values[1], values[2]
	}

	if code >= 16 && code <= 231 {
		index := code - 16
		r := index / 36
		g := (index % 36) / 6
		b := index % 6
		toRGB := func(value int) int {
			if value == 0 {
				return 0
			}
			return 55 + value*40
		}
		return toRGB(r), toRGB(g), toRGB(b)
	}

	if code >= 232 && code <= 255 {
		gray := 8 + (code-232)*10
		return gray, gray, gray
	}

	return 128, 128, 128
}
