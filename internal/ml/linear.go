package ml

// LinearRegression modela un objetivo continuo y = wᵀ·[1,x] y se entrena
// minimizando el error cuadrático medio (MSE). Predice supervivencia
// (días) y costo (USD); ambos objetivos se estandarizan antes de
// entrenar para que el gradiente converja de forma estable.
type LinearRegression struct{ ModelName string }

func NewLinearRegression(name string) *LinearRegression {
	return &LinearRegression{ModelName: name}
}

func (m *LinearRegression) Name() string      { return m.ModelName }
func (m *LinearRegression) IsClassifier() bool { return false }

func (m *LinearRegression) Predict(x, w []float64) float64 {
	return linComb(x, w)
}

// AccumSample: para 0.5·(ŷ-y)², dL/dŷ = (ŷ-y) = r, gradiente w[0]=r,
// w[i+1]=r·x[i].
func (m *LinearRegression) AccumSample(x []float64, y float64, w, gradOut []float64) float64 {
	r := linComb(x, w) - y
	gradOut[0] += r
	for i, xi := range x {
		gradOut[i+1] += r * xi
	}
	return 0.5 * r * r
}
