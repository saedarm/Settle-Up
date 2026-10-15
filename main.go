// Settle Up: a live estimation game. Players join from their phones,
// the host runs the slides, and the server keeps the score.
package main

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

//go:embed web
var webFS embed.FS

type Question struct {
	ID, Cat, Q, Pre, Unit, Ctx, Math, Src, URL string
	A                                          float64
	Derived                                    bool
}

type Player struct {
	ID, Token, Name string
	Answers         map[int]float64
}

type Game struct {
	mu      sync.Mutex
	sid     string
	phase   string // idle, lobby, question, reveal, board, final
	ids     []string
	qi      int
	dur     int
	endsAt  time.Time
	timer   *time.Timer
	paused  bool
	left    time.Duration // time left on the clock while paused
	players map[string]*Player
	order   []string // join order, for stable display
	subs    map[chan []byte]struct{}
}

var (
	game    = &Game{phase: "idle", dur: 45, players: map[string]*Player{}, subs: map[chan []byte]struct{}{}}
	byID    = map[string]Question{}
	hostKey string
)

const grace = 1500 * time.Millisecond

func main() {
	for _, q := range bank {
		byID[q.ID] = q
	}
	hostKey = os.Getenv("HOST_KEY")
	if hostKey == "" {
		hostKey = randHex(4)
		log.Printf("HOST_KEY not set; host screen is /host?key=%s", hostKey)
	}

	web, _ := fs.Sub(webFS, "web")
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(web)))
	mux.HandleFunc("GET /{$}", page(web, "index.html"))
	mux.HandleFunc("GET /host", page(web, "host.html"))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /api/events", events)
	mux.HandleFunc("POST /api/join", join)
	mux.HandleFunc("POST /api/answer", answer)
	mux.HandleFunc("GET /api/host/bank", host(hostBank))
	mux.HandleFunc("POST /api/host/start", host(hostStart))
	mux.HandleFunc("POST /api/host/advance", host(hostAdvance))
	mux.HandleFunc("POST /api/host/end", host(hostEnd))
	mux.HandleFunc("POST /api/host/pause", host(hostPause))
	mux.HandleFunc("POST /api/host/addtime", host(hostAddTime))
	mux.HandleFunc("POST /api/host/restart-question", host(hostRestartQuestion))
	mux.HandleFunc("POST /api/host/restart-game", host(hostRestartGame))
	mux.HandleFunc("POST /api/host/player", host(hostPlayer))
	mux.HandleFunc("GET /api/host/results.csv", host(hostResultsCSV))

	// Re-send state every few seconds during a question so phones stay in sync.
	go func() {
		for range time.Tick(5 * time.Second) {
			game.mu.Lock()
			if game.phase == "question" && !game.paused {
				game.broadcastLocked()
			}
			game.mu.Unlock()
		}
	}()

	addr := ":" + envOr("PORT", "8080")
	log.Printf("Settle Up listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

/* ---------- HTTP helpers ---------- */

func page(web fs.FS, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := fs.ReadFile(web, name)
		if err != nil {
			http.Error(w, "missing page", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(b)
	}
}

func host(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		k := r.Header.Get("X-Host-Key")
		if subtle.ConstantTimeCompare([]byte(k), []byte(hostKey)) != 1 {
			http.Error(w, "wrong host key", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

func readJSON(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 8<<10)
	return json.NewDecoder(r.Body).Decode(v)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

/* ---------- player endpoints ---------- */

func events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")

	ch := make(chan []byte, 8)
	game.mu.Lock()
	game.subs[ch] = struct{}{}
	first := game.stateLocked()
	game.mu.Unlock()
	defer func() { game.mu.Lock(); delete(game.subs, ch); game.mu.Unlock() }()

	w.Write([]byte("retry: 2000\n"))
	writeEvent(w, first)
	fl.Flush()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case b := <-ch:
			writeEvent(w, b)
			fl.Flush()
		case <-ping.C:
			w.Write([]byte(": ping\n\n"))
			fl.Flush()
		}
	}
}

func writeEvent(w http.ResponseWriter, b []byte) {
	w.Write([]byte("data: "))
	w.Write(b)
	w.Write([]byte("\n\n"))
}

func join(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name, PID, Token string }
	if readJSON(r, &in) != nil {
		writeJSON(w, 400, map[string]string{"error": "bad request"})
		return
	}
	name := cleanName(in.Name)
	if name == "" {
		writeJSON(w, 400, map[string]string{"error": "Enter a name."})
		return
	}
	g := game
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.players[in.PID]
	if p == nil || subtle.ConstantTimeCompare([]byte(p.Token), []byte(in.Token)) != 1 {
		if len(g.players) >= 500 {
			writeJSON(w, 503, map[string]string{"error": "The game is full."})
			return
		}
		p = &Player{ID: randHex(6), Token: randHex(16), Answers: map[int]float64{}}
		g.players[p.ID] = p
		g.order = append(g.order, p.ID)
	}
	p.Name = g.uniqueName(name, p.ID)
	g.broadcastLocked()
	writeJSON(w, 200, map[string]string{"pid": p.ID, "token": p.Token, "name": p.Name})
}

func answer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PID, Token, Sid string
		QI              int
		Value           float64
	}
	if readJSON(r, &in) != nil {
		writeJSON(w, 400, map[string]string{"error": "bad request"})
		return
	}
	g := game
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.players[in.PID]
	switch {
	case p == nil || subtle.ConstantTimeCompare([]byte(p.Token), []byte(in.Token)) != 1:
		writeJSON(w, 401, map[string]string{"error": "rejoin"})
	case g.phase != "question" || in.QI != g.qi || in.Sid != g.sid || (!g.paused && time.Now().After(g.endsAt.Add(grace))):
		writeJSON(w, 409, map[string]string{"error": "Time's up for that question."})
	case !(in.Value > 0) || math.IsInf(in.Value, 0) || in.Value > 1e18:
		writeJSON(w, 400, map[string]string{"error": "Enter a positive number."})
	default:
		if _, done := p.Answers[g.qi]; done {
			writeJSON(w, 409, map[string]any{"error": "Already locked in.", "value": p.Answers[g.qi]})
			return
		}
		p.Answers[g.qi] = in.Value
		// Everyone's in: cut the clock to 3 seconds.
		if !g.paused && g.allAnsweredLocked() && time.Until(g.endsAt) > 3*time.Second {
			g.setTimerLocked(3 * time.Second)
		}
		g.broadcastLocked()
		writeJSON(w, 200, map[string]any{"ok": true, "value": in.Value})
	}
}

/* ---------- host endpoints ---------- */

func hostBank(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, bank) }

func hostStart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string
		Dur int
	}
	if readJSON(r, &in) != nil {
		writeJSON(w, 400, map[string]string{"error": "bad request"})
		return
	}
	var ids []string
	for _, id := range in.IDs {
		if _, ok := byID[id]; ok {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		writeJSON(w, 400, map[string]string{"error": "Pick at least one question."})
		return
	}
	if in.Dur < 10 || in.Dur > 300 {
		in.Dur = 45
	}
	g := game
	g.mu.Lock()
	defer g.mu.Unlock()
	g.stopTimerLocked()
	g.sid = randHex(4)
	g.phase, g.ids, g.qi, g.dur, g.paused = "lobby", ids, 0, in.Dur, false
	// Keep players who already joined; wipe their answers.
	for _, p := range g.players {
		p.Answers = map[int]float64{}
	}
	g.broadcastLocked()
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func hostAdvance(w http.ResponseWriter, r *http.Request) {
	g := game
	g.mu.Lock()
	defer g.mu.Unlock()
	g.advanceLocked()
	writeJSON(w, 200, map[string]string{"phase": g.phase})
}

func hostEnd(w http.ResponseWriter, r *http.Request) {
	g := game
	g.mu.Lock()
	defer g.mu.Unlock()
	g.stopTimerLocked()
	g.phase, g.ids, g.qi, g.paused = "idle", nil, 0, false
	g.broadcastLocked()
	writeJSON(w, 200, map[string]bool{"ok": true})
}

/* ---------- game logic (call with g.mu held) ---------- */

func (g *Game) advanceLocked() {
	n := len(g.ids)
	g.paused = false
	switch g.phase {
	case "lobby", "board":
		if g.phase == "board" {
			g.qi++
		}
		g.phase = "question"
		g.setTimerLocked(time.Duration(g.dur) * time.Second)
	case "question":
		g.stopTimerLocked()
		if time.Now().Before(g.endsAt) {
			g.endsAt = time.Now()
		}
		g.phase = "reveal"
	case "reveal":
		if g.qi >= n-1 {
			g.phase = "final"
		} else {
			g.phase = "board"
		}
	default:
		return
	}
	g.broadcastLocked()
}

func (g *Game) setTimerLocked(d time.Duration) {
	g.stopTimerLocked()
	g.endsAt = time.Now().Add(d)
	sid, qi := g.sid, g.qi
	g.timer = time.AfterFunc(d+grace, func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.phase == "question" && g.sid == sid && g.qi == qi {
			g.advanceLocked()
		}
	})
	g.broadcastLocked()
}

func (g *Game) stopTimerLocked() {
	if g.timer != nil {
		g.timer.Stop()
		g.timer = nil
	}
}

func (g *Game) allAnsweredLocked() bool {
	if len(g.players) == 0 {
		return false
	}
	for _, p := range g.players {
		if _, ok := p.Answers[g.qi]; !ok {
			return false
		}
	}
	return true
}

func (g *Game) uniqueName(name, self string) string {
	taken := map[string]bool{}
	for id, p := range g.players {
		if id != self {
			taken[strings.ToLower(p.Name)] = true
		}
	}
	out := name
	for i := 2; taken[strings.ToLower(out)]; i++ {
		out = name + " " + strconv.Itoa(i)
	}
	return out
}

func points(r float64) int {
	if math.IsInf(r, 0) || r >= 100 {
		return 0
	}
	return int(math.Round(1000 * (1 - math.Log10(r)/2)))
}

func ratio(g, a float64) float64 {
	if !(g > 0) {
		return math.Inf(1)
	}
	return math.Max(g/a, a/g)
}

/* ---------- public state ---------- */

type pubQ struct {
	ID, Cat, Q, Pre, Unit, Ctx string
}
type pubGuess struct {
	PID   string  `json:"pid"`
	Name  string  `json:"name"`
	G     float64 `json:"g"`
	Ratio float64 `json:"ratio"`
	Pts   int     `json:"pts"`
}
type pubReveal struct {
	A       float64    `json:"a"`
	Math    string     `json:"math"`
	Src     string     `json:"src"`
	URL     string     `json:"url"`
	Derived bool       `json:"derived"`
	Guesses []pubGuess `json:"guesses"`
}
type pubRow struct {
	PID      string `json:"pid"`
	Name     string `json:"name"`
	Total    int    `json:"total"`
	Rank     int    `json:"rank"`
	Answered bool   `json:"answered"`
}
type pubState struct {
	Sid         string     `json:"sid"`
	Phase       string     `json:"phase"`
	QI          int        `json:"qi"`
	Total       int        `json:"total"`
	Dur         int        `json:"dur"`
	RemainingMs int64      `json:"remainingMs"`
	Paused      bool       `json:"paused"`
	Question    *pubQ      `json:"question,omitempty"`
	Reveal      *pubReveal `json:"reveal,omitempty"`
	Board       []pubRow   `json:"board"`
	Answered    int        `json:"answered"`
}

func (g *Game) stateLocked() []byte {
	s := pubState{Sid: g.sid, Phase: g.phase, QI: g.qi, Total: len(g.ids), Dur: g.dur, Board: []pubRow{}}
	if g.phase == "question" {
		s.Paused = g.paused
		if g.paused {
			s.RemainingMs = g.left.Milliseconds()
		} else {
			s.RemainingMs = max(0, time.Until(g.endsAt).Milliseconds())
		}
	}
	var q Question
	if g.phase != "idle" && g.qi < len(g.ids) {
		q = byID[g.ids[g.qi]]
		if g.phase == "question" || g.phase == "reveal" {
			s.Question = &pubQ{q.ID, q.Cat, q.Q, q.Pre, q.Unit, q.Ctx}
		}
	}
	// Scores through the last question that has been revealed.
	upto := g.qi - 1
	if g.phase == "reveal" || g.phase == "board" || g.phase == "final" {
		upto = g.qi
	}
	type acc struct {
		row pubRow
		err float64
	}
	var rows []acc
	for _, id := range g.order {
		p := g.players[id]
		a := acc{row: pubRow{PID: p.ID, Name: p.Name}}
		for i := 0; i <= upto && i < len(g.ids); i++ {
			if v, ok := p.Answers[i]; ok {
				r := ratio(v, byID[g.ids[i]].A)
				a.row.Total += points(r)
				a.err += math.Log10(r)
			} else {
				a.err += 2
			}
		}
		_, a.row.Answered = p.Answers[g.qi]
		if a.row.Answered {
			s.Answered++
		}
		rows = append(rows, a)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].row.Total != rows[j].row.Total {
			return rows[i].row.Total > rows[j].row.Total
		}
		return rows[i].err < rows[j].err
	})
	for i := range rows {
		rows[i].row.Rank = i + 1
		s.Board = append(s.Board, rows[i].row)
	}
	if g.phase == "reveal" {
		rv := &pubReveal{A: q.A, Math: q.Math, Src: q.Src, URL: q.URL, Derived: q.Derived, Guesses: []pubGuess{}}
		for _, id := range g.order {
			p := g.players[id]
			if v, ok := p.Answers[g.qi]; ok {
				r := ratio(v, q.A)
				rv.Guesses = append(rv.Guesses, pubGuess{p.ID, p.Name, v, r, points(r)})
			}
		}
		sort.SliceStable(rv.Guesses, func(i, j int) bool { return rv.Guesses[i].Ratio < rv.Guesses[j].Ratio })
		s.Reveal = rv
	}
	b, _ := json.Marshal(s)
	return b
}

func (g *Game) broadcastLocked() {
	b := g.stateLocked()
	for ch := range g.subs {
		select {
		case ch <- b:
		default: // slow client; it gets the next update
		}
	}
}

/* ---------- small utils ---------- */

func cleanName(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		b.WriteRune(r)
		if n++; n >= 24 {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
