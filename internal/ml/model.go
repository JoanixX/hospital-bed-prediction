// Package ml implementa un motor de entrenamiento de modelos lineales
// generalizados por descenso de gradiente, con paralelismo intra-nodo
// (goroutines) y agregación distribuida de gradientes (map-reduce entre
// nodos). El diseño se basa en una interfaz Model intercambiable, de modo
// que el mismo motor entrena clasificación logística y regresión lineal,
// y es extensible a softmax/MLP sin tocar el coordinador distribuido.
package ml

import "math"

// Model abstrae un modelo lineal generalizado y = f(wᵀ·[1,x]).
// Un vector de pesos w tiene longitud D+1: w[0] es el sesgo (bias) y
// w[1..D] los coeficientes de las D features.
type Model interface {
	// Name identifica el modelo en logs y reportes.
	Name() string
	// IsClassifier indica si la salida es una probabilidad [0,1].
	IsClassifier() bool
	// Predict devuelve la salida del modelo para una muestra x dado w.
	Predict(x, w []float64) float64
	// AccumSample acumula en gradOut el gradiente de la pérdida de UNA
	// muestra (x, y) respecto a w, y devuelve la pérdida de esa muestra.
	// No promedia ni aplica regularización: eso lo hace el agregador.
	AccumSample(x []float64, y float64, w, gradOut []float64) float64
}

// linComb calcula z = w[0] + Σ w[i+1]·x[i].
func linComb(x, w []float64) float64 {
	z := w[0]
	for i, xi := range x {
		z += w[i+1] * xi
	}
	return z
}

func sigmoid(z float64) float64 {
	if z >= 0 {
		return 1.0 / (1.0 + math.Exp(-z))
	}
	ez := math.Exp(z)
	return ez / (1.0 + ez)
}

// Grad es el resultado de acumular gradientes sobre un conjunto de
// muestras: Sum es la suma (sin promediar) de los gradientes por muestra,
// Loss la suma de pérdidas y N el número de muestras. Es el "mensaje"
// que cada worker devuelve al master en el entrenamiento distribuido.
type Grad struct {
	Sum  []float64
	Loss float64
	N    int
}

// NewGrad crea un acumulador de dimensión dim (= D+1).
func NewGrad(dim int) Grad { return Grad{Sum: make([]float64, dim), N: 0} }

// Add suma otro Grad (agregación map-reduce entre shards/goroutines).
func (g *Grad) Add(o Grad) {
	if len(g.Sum) == 0 {
		g.Sum = make([]float64, len(o.Sum))
	}
	for i := range o.Sum {
		g.Sum[i] += o.Sum[i]
	}
	g.Loss += o.Loss
	g.N += o.N
}

// MeanGradient devuelve el gradiente promedio (Sum/N) y le añade el
// término de regularización L2 (λ·w) sobre los pesos NO-sesgo. Es el
// paso final, idéntico se haga centralizado o distribuido.
func (g Grad) MeanGradient(l2 float64, w []float64) []float64 {
	out := make([]float64, len(g.Sum))
	n := float64(g.N)
	if n == 0 {
		n = 1
	}
	for i := range g.Sum {
		out[i] = g.Sum[i] / n
	}
	for i := 1; i < len(out); i++ { // L2 no aplica al sesgo w[0]
		out[i] += l2 * w[i]
	}
	return out
}

// MeanLoss devuelve la pérdida media sobre las muestras acumuladas.
func (g Grad) MeanLoss() float64 {
	if g.N == 0 {
		return 0
	}
	return g.Loss / float64(g.N)
}
