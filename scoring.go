package main

import "math"

// Scoring works in bands: land within a band, get its points.
//
// Typed answers are judged by how many TIMES off the guess is, so guessing
// half the answer counts the same as guessing double it.
// Slider answers are judged by distance, as a share of the slider's range.

type band struct {
	Within float64 // typed: max "times off"; slider: max distance as a fraction of the range
	Points int
}

var typedBands = []band{
	{1.10, 1000}, // within 10%
	{1.25, 900},
	{1.5, 800},
	{2, 700}, // half or double
	{3, 550},
	{5, 400},
	{10, 250}, // right order of magnitude
	{100, 50}, // at least in the neighborhood
}

var sliderBands = []band{
	{0.02, 1000}, // within 2 on a 0-100 slider
	{0.05, 800},
	{0.10, 600},
	{0.15, 450},
	{0.25, 300},
	{0.40, 150},
}

type sliderSpec struct{ Min, Max, Step float64 }

// Questions with a natural range get a slider instead of a number box.
var sliders = map[string]sliderSpec{
	"gp_ctry":     {0, 195, 1},
	"gp_ticket":   {0, 200, 1},
	"gp_perloc":   {0, 200, 1},
	"gpr_us":      {0, 100, 1},
	"gpr_cn":      {0, 100, 1},
	"gpr_pix":     {0, 100, 1},
	"cpt_agentic": {0, 100, 1},
	"cpt_ai":      {0, 100, 1},
	"cpt_mcd":     {0, 100, 1},
	"cpt_wallet":  {0, 100, 1},
	"cpt_usd":     {0, 100, 1},
	"cpt_micro":   {0, 100, 1},
	"cpt_steak":   {0, 20, 1},
	"hw_pp":       {0, 300, 5},
	"tg_dinner":   {0, 200, 1},
}

// score returns the points for a guess and an error measure used to break ties
// and to pick the closest guess (lower is closer).
func score(q Question, g float64) (int, float64) {
	if q.Kind == "slider" {
		d := math.Abs(g-q.A) / (q.Max - q.Min)
		for _, b := range sliderBands {
			if d <= b.Within+1e-9 {
				return b.Points, d
			}
		}
		return 0, d
	}
	r := ratio(g, q.A)
	e := math.Log10(r)
	if math.IsInf(r, 0) {
		return 0, 10
	}
	for _, b := range typedBands {
		if r <= b.Within+1e-9 {
			return b.Points, e
		}
	}
	return 0, e
}

func ratio(g, a float64) float64 {
	if !(g > 0) {
		return math.Inf(1)
	}
	return math.Max(g/a, a/g)
}

func validGuess(q Question, v float64) bool {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return false
	}
	if q.Kind == "slider" {
		return v >= q.Min && v <= q.Max
	}
	return v > 0 && v <= 1e18
}

func first(a int, _ float64) int { return a }
