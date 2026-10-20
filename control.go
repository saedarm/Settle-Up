package main

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"time"
)

// Host controls beyond "advance": pause, more time, do-overs, player moderation, export.

// POST /api/host/pause toggles the clock during a question.
func hostPause(w http.ResponseWriter, r *http.Request) {
	g := game
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "question" {
		writeJSON(w, 409, map[string]string{"error": "Pause only works during a question."})
		return
	}
	if g.paused {
		g.paused = false
		g.setTimerLocked(max(g.left, 3*time.Second))
	} else {
		g.left = max(0, time.Until(g.endsAt))
		g.stopTimerLocked()
		g.paused = true
		g.broadcastLocked()
	}
	writeJSON(w, 200, map[string]bool{"paused": g.paused})
}

// POST /api/host/addtime adds 15 seconds to the running (or paused) clock.
func hostAddTime(w http.ResponseWriter, r *http.Request) {
	g := game
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "question" {
		writeJSON(w, 409, map[string]string{"error": "Only during a question."})
		return
	}
	if g.paused {
		g.left += 15 * time.Second
		g.broadcastLocked()
	} else {
		g.setTimerLocked(max(0, time.Until(g.endsAt)) + 15*time.Second)
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// POST /api/host/restart-question wipes everyone's answer to the current
// question and runs its clock again from the top.
func hostRestartQuestion(w http.ResponseWriter, r *http.Request) {
	g := game
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "question" && g.phase != "reveal" && g.phase != "board" {
		writeJSON(w, 409, map[string]string{"error": "No question to restart."})
		return
	}
	for _, p := range g.players {
		delete(p.Answers, g.qi)
	}
	g.paused = false
	g.phase = "question"
	g.sid = randHex(4) // phones forget their locked-in guess for the new round
	g.setTimerLocked(time.Duration(g.dur) * time.Second)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// POST /api/host/restart-game reshuffles nothing: same questions, scores wiped, back to the lobby.
func hostRestartGame(w http.ResponseWriter, r *http.Request) {
	g := game
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.ids) == 0 {
		writeJSON(w, 409, map[string]string{"error": "No game to restart."})
		return
	}
	g.stopTimerLocked()
	for _, p := range g.players {
		p.Answers = map[int]float64{}
	}
	g.sid, g.phase, g.qi, g.paused = randHex(4), "lobby", 0, false
	g.broadcastLocked()
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// POST /api/host/player {pid, action: "remove"|"rename", name}
func hostPlayer(w http.ResponseWriter, r *http.Request) {
	var in struct{ PID, Action, Name string }
	if readJSON(r, &in) != nil {
		writeJSON(w, 400, map[string]string{"error": "bad request"})
		return
	}
	g := game
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.players[in.PID]
	if p == nil {
		writeJSON(w, 404, map[string]string{"error": "No such player."})
		return
	}
	switch in.Action {
	case "remove":
		g.removed[p.ID] = p.Token
		delete(g.players, p.ID)
		for i, id := range g.order {
			if id == p.ID {
				g.order = append(g.order[:i], g.order[i+1:]...)
				break
			}
		}
	case "rename":
		n := cleanName(in.Name)
		if n == "" {
			writeJSON(w, 400, map[string]string{"error": "Enter a name."})
			return
		}
		p.Name = g.uniqueName(n, p.ID)
	default:
		writeJSON(w, 400, map[string]string{"error": "Unknown action."})
		return
	}
	g.broadcastLocked()
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// GET /api/host/results.csv: rank, name, total, then each question's guess and points.
func hostResultsCSV(w http.ResponseWriter, r *http.Request) {
	g := game
	g.mu.Lock()
	defer g.mu.Unlock()
	var st pubState
	if err := jsonUnmarshal(g.stateLocked(), &st); err != nil {
		http.Error(w, "export failed", 500)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="settle-up-results.csv"`)
	cw := csv.NewWriter(w)
	head := []string{"Rank", "Name", "Total points"}
	for i, id := range g.ids {
		head = append(head, "Q"+strconv.Itoa(i+1)+" guess ("+id+")", "Q"+strconv.Itoa(i+1)+" points")
	}
	cw.Write(head)
	for _, row := range st.Board {
		p := g.players[row.PID]
		rec := []string{strconv.Itoa(row.Rank), row.Name, strconv.Itoa(row.Total)}
		for i, id := range g.ids {
			if v, ok := p.Answers[i]; ok {
				rec = append(rec, strconv.FormatFloat(v, 'g', 6, 64), strconv.Itoa(first(score(byID[id], v))))
			} else {
				rec = append(rec, "", "0")
			}
		}
		cw.Write(rec)
	}
	cw.Flush()
}
