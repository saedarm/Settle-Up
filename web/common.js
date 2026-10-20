// Shared helpers for the phone and host pages.
const $ = s => document.querySelector(s);
const esc = s => String(s ?? "").replace(/[&<>"']/g, c => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));
const store = {
  get(k){ try { return JSON.parse(localStorage.getItem(k)); } catch(e){ return null; } },
  set(k,v){ try { localStorage.setItem(k, JSON.stringify(v)); } catch(e){} }
};
const CATS = {pay:"Global Payments", trends:"Trends report", holiday:"Holiday"};

// "94b/365", "6m*40", "$3.7 trillion" -> number. null = empty, NaN = unreadable.
function parseGuess(raw){
  let s = String(raw).toLowerCase().replace(/[$,_\s%]/g, "");
  if (!s) return null;
  s = s.replace(/(\d*\.?\d+)(trillion|tn|t|billion|bn|b|million|mm|m|thousand|k)(?![a-z])/g,
    (m,n,u) => "(" + n + ({trillion:"e12",tn:"e12",t:"e12",billion:"e9",bn:"e9",b:"e9",million:"e6",mm:"e6",m:"e6",thousand:"e3",k:"e3"}[u]) + ")");
  s = s.replace(/x/g, "*").replace(/÷/g, "/");
  if (!/^[0-9+\-*/().e]+$/.test(s)) return NaN;
  try { const v = Function('"use strict";return (' + s + ')')(); return (typeof v === "number" && isFinite(v)) ? v : NaN; }
  catch(e){ return NaN; }
}
function fmt(v, pre=""){
  const a = Math.abs(v), sig = x => (+x.toFixed(x>=100?0:x>=10?1:2)).toLocaleString("en-US");
  let o;
  if (a>=1e12) o = sig(v/1e12)+" trillion"; else if (a>=1e9) o = sig(v/1e9)+" billion";
  else if (a>=1e6) o = sig(v/1e6)+" million"; else if (a>=100) o = Math.round(v).toLocaleString("en-US");
  else o = (+v.toFixed(a<10?2:1)).toLocaleString("en-US");
  return pre + o;
}
const rfmt = r => !isFinite(r) ? "—" : (r>=100 ? Math.round(r).toLocaleString("en-US") : r>=10 ? r.toFixed(1) : r.toFixed(2)) + "×";
const dir = (g,a) => { const r = Math.max(g/a, a/g); return r<1.1 ? "good" : g<a ? "low" : "high"; };
const dirWord = (g,a) => { const r = Math.max(g/a, a/g); return r<1.1 ? "Nailed it" : g<a ? "too low" : "too high"; };
const pos = (x,a) => Math.min(98, Math.max(2, (Math.log10(x/a)+2)/4*100));
const chip = c => `<span class="chip ${c}">${CATS[c]||""}</span>`;
const ordinal = n => { const s=["th","st","nd","rd"], v=n%100; return n+(s[(v-20)%10]||s[v]||s[0]); };
function srcLine(rv){
  const name = esc(rv.src);
  return `<p class="src">Source: ${rv.url ? `<a href="${esc(rv.url)}" target="_blank" rel="noopener">${name}</a>` : name}${rv.derived ? " (answer derived from the published figures)" : ""}</p>`;
}
function strip(a, guesses, you){
  const ticks = [0,25,50,75,100].map(p => `<span class="tick" style="left:${p}%"></span>`).join("");
  const dots = (guesses||[]).map(g => `<span class="dotg" style="left:${pos(g,a)}%"></span>`).join("");
  return `<div aria-label="Guesses compared with the answer on a log scale"><div class="track">${ticks}${dots}<span class="mark ans" style="left:50%"><i></i></span>${you ? `<span class="mark you" style="left:${pos(you,a)}%">YOU<i></i></span>` : ""}</div>
  <div class="scale"><span>÷100</span><span>÷10</span><span>answer</span><span>×10</span><span>×100</span></div></div>`;
}
async function post(url, body, headers={}){
  const r = await fetch(url, {method:"POST", headers:{"Content-Type":"application/json", ...headers}, body:JSON.stringify(body||{})});
  let j = {}; try { j = await r.json(); } catch(e){}
  return {ok:r.ok, status:r.status, ...j};
}
// Live state over Server-Sent Events. onState gets (state, localEndTime).
function subscribe(onState, onConn){
  let es;
  const open = () => {
    es = new EventSource("/api/events");
    es.onopen = () => onConn && onConn(true);
    es.onmessage = e => { try { const s = JSON.parse(e.data); onState(s, Date.now() + (s.remainingMs||0)); } catch(err){} };
    es.onerror = () => {
      onConn && onConn(false);
      // Browsers retry on their own, but some give up (readyState 2 = closed). Reopen ourselves.
      if (es.readyState === 2) setTimeout(() => { if (es.readyState === 2) open(); }, 1500);
    };
  };
  open();
  // Phones drop connections when the screen locks; reopen on wake.
  document.addEventListener("visibilitychange", () => {
    if (!document.hidden && es && es.readyState === 2) open();
  });
}

// ---- harvest decor: pumpkin in the wordmark, a few drifting leaves ----
const PUMPKIN = `<svg viewBox="0 0 32 32" aria-hidden="true"><path d="M16 8c1-3 3-4 5-4" stroke="#8fd16a" stroke-width="2.4" fill="none" stroke-linecap="round"/><ellipse cx="10" cy="19" rx="7" ry="9" fill="#d96f17"/><ellipse cx="22" cy="19" rx="7" ry="9" fill="#d96f17"/><ellipse cx="16" cy="19" rx="7" ry="10" fill="#f08a24"/><path d="M16 10v18" stroke="#c45f10" stroke-width="1.2" opacity=".6"/></svg>`;
function leafURI(c){ return `url("data:image/svg+xml,${encodeURIComponent(`<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24'><path fill='${c}' d='M12 2l2 4 4-1-1 4 4 2-4 2 1 4-4-1-2 4-2-4-4 1 1-4-4-2 4-2-1-4 4 1z'/><path d='M12 12v10' stroke='${c}' stroke-width='1.5'/></svg>`)}")`; }
function decorate(count){
  document.querySelectorAll(".brand").forEach(b => { if (!b.querySelector("svg")) b.insertAdjacentHTML("afterbegin", PUMPKIN); });
  const box = document.createElement("div"); box.className = "leaves"; box.setAttribute("aria-hidden", "true");
  const colors = ["#f08a24", "#e2546b", "#f2c14e", "#c45f10"];
  for (let i = 0; i < count; i++){
    const l = document.createElement("i");
    l.style.left = (Math.random()*100) + "%";
    l.style.setProperty("--leaf", leafURI(colors[i % colors.length]));
    l.style.setProperty("--dx", (Math.random()*160 - 80) + "px");
    l.style.animationDuration = (14 + Math.random()*14) + "s";
    l.style.animationDelay = (-Math.random()*28) + "s";
    const s = 14 + Math.random()*14; l.style.width = l.style.height = s + "px";
    box.appendChild(l);
  }
  document.body.prepend(box);
}
function toast(msg){
  const t = document.createElement("div"); t.className = "toast"; t.textContent = msg; t.setAttribute("role", "status");
  document.body.appendChild(t); setTimeout(() => t.remove(), 2200);
}

// ---- sliders and size picker ----
const SCALES = [[1, "units"], [1e3, "thousand"], [1e6, "million"], [1e9, "billion"], [1e12, "trillion"]];
// True when the typed text already says how big it is ("13b", "6 million").
const hasSuffix = s => /\d\s*(k|m|mm|b|bn|t|tn|thousand|million|billion|trillion)\b/i.test(String(s));
const isSlider = q => q && q.Kind === "slider";
function stripLin(a, min, max, guesses, you){
  const p = x => Math.min(98, Math.max(2, (x - min) / (max - min) * 100));
  const ticks = [0,25,50,75,100].map(v => `<span class="tick" style="left:${v}%"></span>`).join("");
  const dots = (guesses||[]).map(g => `<span class="dotg" style="left:${p(g)}%"></span>`).join("");
  const lab = v => (+v.toFixed(1)).toLocaleString("en-US");
  return `<div aria-label="Guesses compared with the answer"><div class="track">${ticks}${dots}<span class="mark ans" style="left:${p(a)}%"><i></i></span>${you !== undefined ? `<span class="mark you" style="left:${p(you)}%">YOU<i></i></span>` : ""}</div>
  <div class="scale"><span>${lab(min)}</span><span>${lab(min+(max-min)/4)}</span><span>${lab(min+(max-min)/2)}</span><span>${lab(min+3*(max-min)/4)}</span><span>${lab(max)}</span></div></div>`;
}
// Strip for either kind of question.
function stripFor(q, a, guesses, you){ return isSlider(q) ? stripLin(a, q.Min, q.Max, guesses, you) : strip(a, guesses, you); }
// "1.05× too high" for typed answers, "off by 3" for sliders.
function offText(q, g, a){
  if (isSlider(q)){
    const d = Math.abs(g - a), close = d / (q.Max - q.Min) <= 0.02;
    return close ? "Nailed it" : `off by ${fmt(d, q.Pre)} · ${g < a ? "too low" : "too high"}`;
  }
  return `${rfmt(Math.max(g/a, a/g))} ${dirWord(g, a)}`;
}
function dirFor(q, g, a){
  if (isSlider(q)) return Math.abs(g - a) / (q.Max - q.Min) <= 0.02 ? "good" : g < a ? "low" : "high";
  return dir(g, a);
}
