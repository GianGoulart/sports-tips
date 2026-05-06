package engine

type KellyResult struct {
	Edge        float64
	ImpliedProb float64
	KellyFull   float64
	KellyHalf   float64
	IsValueBet  bool
}

func Kelly(odds, modelProb float64) KellyResult {
	if odds <= 1.0 || modelProb <= 0 || modelProb >= 1 {
		return KellyResult{}
	}
	impliedProb := 1.0 / odds
	edge := modelProb - impliedProb

	r := KellyResult{
		Edge:        edge,
		ImpliedProb: impliedProb,
	}

	if edge <= 0 {
		return r
	}

	b := odds - 1.0
	q := 1.0 - modelProb
	kellyFull := (b*modelProb - q) / b
	if kellyFull < 0 {
		return r
	}

	r.KellyFull = kellyFull
	r.KellyHalf = kellyFull * 0.5
	r.IsValueBet = true
	return r
}
