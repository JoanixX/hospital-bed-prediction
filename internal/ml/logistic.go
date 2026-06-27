package ml

import "math"

// LogisticRegression modela la probabilidad de un evento binario
// P(y=1|x) = σ(wᵀ·[1,x]) y se entrena minimizando la log-loss
// (entropía cruzada binaria). En este proyecto predice la mortalidad.
type LogisticRegression struct{ ModelName string }

func NewLogisticRegression(name string) *LogisticRegression {
	return &LogisticRegression{ModelName: name}
}

func (m *LogisticRegression) Name() string      { return m.ModelName }
func (m *LogisticRegression) IsClassifier() bool { return true }

func (m *LogisticRegression) Predict(x, w []float64) float64 {
	return sigmoid(linComb(x, w))
}

// AccumSample: para log-loss, dL/dz = (p - y), por lo que el gradiente
// respecto a w[0] es (p-y) y respecto a w[i+1] es (p-y)·x[i].
func (m *LogisticRegression) AccumSample(x []float64, y float64, w, gradOut []float64) float64 {
	p := sigmoid(linComb(x, w))
	d := p - y
	gradOut[0] += d
	for i, xi := range x {
		gradOut[i+1] += d * xi
	}
	// Log-loss con recorte numérico para evitar log(0).
	const eps = 1e-12
	pp := math.Min(math.Max(p, eps), 1-eps)
	return -(y*math.Log(pp) + (1-y)*math.Log(1-pp))
}
