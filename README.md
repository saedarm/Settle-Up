# Settle Up

A live estimation game for the Global Payments Thanksgiving/Halloween breakfast.
Everyone plays on their phone at the main URL; the host runs the slides at `/host`.
No accounts, no sign-in.

One Go binary, no dependencies. The question bank is in `questions.go`, and answers
never leave the server until each reveal.

## Run it locally

    HOST_KEY=pick-a-secret go run .

- Players: http://localhost:8080
- Host:    http://localhost:8080/host?key=pick-a-secret

If you leave `HOST_KEY` unset, the server makes one up and prints it at startup.

## Deploy to Fly.io

    fly launch --no-deploy --copy-config --name settleup-gp   # pick any free app name
    fly secrets set HOST_KEY=pick-a-secret
    fly deploy
    fly scale count 1    # the game lives in memory: exactly one machine

Then open `https://<your-app>.fly.dev/host?key=pick-a-secret` on the projector laptop.
The lobby shows a QR code and the address for phones.

The game state is in memory, so a redeploy or restart wipes the current game.
Don't deploy during the breakfast. Players who were mid-game will be asked for
their name again and can keep playing from the next question.

## Running the game

1. Setup: tick the questions you want (or "Pick a fresh 10-question mix") and the time per question.
2. Lobby: names appear as people join. Press Space or → (or the button) to start.
3. Each question ends when the clock runs out. If everyone has answered, the clock drops to 3 seconds.
4. Reveal → Standings → next question. After the last one, the podium and a "Download results" button.

### Host menu (top right) and shortcuts

| Control | Shortcut | What it does |
|---|---|---|
| Next step | Space or → | Start / reveal / standings / next question |
| Pause / Resume the clock | P | Freezes the timer on every screen. Phones can still lock in. |
| Add 15 seconds | + | Works running or paused |
| Restart this question | | Wipes everyone's guess for the current question and restarts its clock |
| Players | | Rename or remove anyone. Removed players go back to the join screen. |
| Download results (CSV) | | Rank, name, total, and each guess with its points |
| Restart the game | | Same questions, all scores wiped, back to the lobby |
| End game and go to setup | | Back to question picking; players stay joined |

The restart and end buttons ask for a second click so they can't be hit by accident.

Scoring: `1000 × (1 − log10(off-by multiple) / 2)`. Dead on = 1,000, 2× off = 849,
10× off = 500, 100× off or worse = 0. Ties go to whoever was closer overall.

## Editing questions

Each entry in `questions.go` has the question, answer (`A`), prefix/unit shown in the
guess box, a hint (`Ctx`), the napkin math shown at the reveal, and its source.
`Cat` is `pay`, `trends` or `holiday`.

## Files

- `main.go`: server, game state, scoring, live updates (Server-Sent Events)
- `questions.go`: question bank
- `web/`: phone page, host page, shared CSS/JS, bundled QR code library (MIT, Kazuhiko Arase)
