package ml

import (
	"math"
	"sort"
)

// ClassMetrics agrupa métricas de clasificación binaria.
type ClassMetrics struct {
	Accuracy float64
	AUC      float64
	LogLoss  float64
}

// EvalClassification evalúa un clasificador sobre (X,y) con pesos w.
func EvalClassification(m Model, X [][]float64, y, w []float64) ClassMetrics {
	n := len(X)
	if n == 0 {
		return ClassMetrics{}
	}
	probs := make([]float64, n)
	correct := 0
	var loss float64
	const eps = 1e-12
	for i := range X {
		p := m.Predict(X[i], w)
		probs[i] = p
		pred := 0.0
		if p >= 0.5 {
			pred = 1.0
		}
		if pred == y[i] {
			correct++
		}
		pp := math.Min(math.Max(p, eps), 1-eps)
		loss += -(y[i]*math.Log(pp) + (1-y[i])*math.Log(1-pp))
	}
	return ClassMetrics{
		Accuracy: float64(correct) / float64(n),
		AUC:      auc(probs, y),
		LogLoss:  loss / float64(n),
	}
}

// auc calcula el área bajo la curva ROC vía el estadístico de
// Mann-Whitney U (equivalente a la probabilidad de rankear un positivo
// por encima de un negativo).
func auc(scores, y []float64) float64 {
	type pair struct {
		s float64
		y float64
	}
	n := len(scores)
	ps := make([]pair, n)
	for i := range scores {
		ps[i] = pair{scores[i], y[i]}
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].s < ps[j].s })

	// Rangos con manejo de empates (rango promedio).
	ranks := make([]float64, n)
	i := 0
	for i < n {
		j := i
		for j < n && ps[j].s == ps[i].s {
			j++
		}
		avg := float64(i+j+1) / 2.0 // rango promedio (1-indexado)
		for k := i; k < j; k++ {
			ranks[k] = avg
		}
		i = j
	}

	var sumRankPos float64
	var nPos, nNeg int
	for k := 0; k < n; k++ {
		if ps[k].y == 1.0 {
			sumRankPos += ranks[k]
			nPos++
		} else {
			nNeg++
		}
	}
	if nPos == 0 || nNeg == 0 {
		return 0.5 // indefinido: clase única
	}
	u := sumRankPos - float64(nPos*(nPos+1))/2.0
	return u / float64(nPos*nNeg)
}

// RegMetrics agrupa métricas de regresión en la escala ORIGINAL del
// objetivo (días, USD), no en la estandarizada.
type RegMetrics struct {
	RMSE float64
	MAE  float64
	R2   float64
}

// EvalRegression evalúa un regresor. preds e yTrue deben venir ya
// de-estandarizados (escala real).
func EvalRegression(preds, yTrue []float64) RegMetrics {
	n := len(yTrue)
	if n == 0 {
		return RegMetrics{}
	}
	var sumSq, sumAbs, mean float64
	for _, v := range yTrue {
		mean += v
	}
	mean /= float64(n)
	var ssRes, ssTot float64
	for i := range yTrue {
		e := preds[i] - yTrue[i]
		sumSq += e * e
		sumAbs += math.Abs(e)
		ssRes += e * e
		d := yTrue[i] - mean
		ssTot += d * d
	}
	r2 := 0.0
	if ssTot > 0 {
		r2 = 1 - ssRes/ssTot
	}
	return RegMetrics{
		RMSE: math.Sqrt(sumSq / float64(n)),
		MAE:  sumAbs / float64(n),
		R2:   r2,
	}
}
