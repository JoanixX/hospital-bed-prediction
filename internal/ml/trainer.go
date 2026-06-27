package ml

import "math/rand"

// TrainConfig parametriza el entrenamiento por descenso de gradiente.
type TrainConfig struct {
	Epochs     int     // número de pasadas completas (épocas)
	LR         float64 // tasa de aprendizaje
	L2         float64 // coeficiente de regularización L2 (ridge)
	NumWorkers int     // goroutines para el gradiente (0 => NumCPU)
	Verbose    bool    // imprime la pérdida por época
	LogEvery   int     // cada cuántas épocas loguear (si Verbose)
}

// DefaultTrainConfig devuelve hiperparámetros razonables para datos
// estandarizados.
func DefaultTrainConfig() TrainConfig {
	return TrainConfig{Epochs: 100, LR: 0.5, L2: 1e-4, NumWorkers: 0, Verbose: false, LogEvery: 10}
}

// TrainResult contiene los pesos aprendidos y la curva de pérdida.
type TrainResult struct {
	Weights     []float64
	LossHistory []float64
}

// TrainLocal entrena un modelo en un solo proceso con gradiente
// descendiente full-batch, paralelizando cada época con goroutines.
// Es la versión "monolítica"; la distribuida (master/worker) usa las
// mismas primitivas (RawGradientConcurrent + MeanGradient).
func TrainLocal(m Model, X [][]float64, y []float64, cfg TrainConfig) TrainResult {
	dim := len(X[0]) + 1
	w := make([]float64, dim) // init en ceros
	hist := make([]float64, 0, cfg.Epochs)

	for epoch := 0; epoch < cfg.Epochs; epoch++ {
		g := RawGradientConcurrent(m, X, y, w, cfg.NumWorkers)
		grad := g.MeanGradient(cfg.L2, w)
		for i := range w {
			w[i] -= cfg.LR * grad[i]
		}
		hist = append(hist, g.MeanLoss())
	}
	return TrainResult{Weights: w, LossHistory: hist}
}

// TrainTestSplit baraja índices con semilla fija y separa una fracción
// de test, devolviendo subconjuntos de X y de un objetivo y.
func TrainTestSplit(X [][]float64, y []float64, testFrac float64, seed int64) (xtr [][]float64, ytr []float64, xte [][]float64, yte []float64) {
	n := len(X)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	r := rand.New(rand.NewSource(seed))
	r.Shuffle(n, func(i, j int) { idx[i], idx[j] = idx[j], idx[i] })
	nTest := int(float64(n) * testFrac)
	for k, id := range idx {
		if k < nTest {
			xte = append(xte, X[id])
			yte = append(yte, y[id])
		} else {
			xtr = append(xtr, X[id])
			ytr = append(ytr, y[id])
		}
	}
	return
}
