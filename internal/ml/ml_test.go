package ml

import (
	"math"
	"math/rand"
	"testing"
)

// makeBinary genera datos linealmente separables con ruido para
// clasificación.
func makeBinary(n int, seed int64) ([][]float64, []float64) {
	r := rand.New(rand.NewSource(seed))
	X := make([][]float64, n)
	y := make([]float64, n)
	for i := 0; i < n; i++ {
		a := r.NormFloat64()
		b := r.NormFloat64()
		X[i] = []float64{a, b}
		z := 1.5*a - 2.0*b + 0.5
		p := 1.0 / (1.0 + math.Exp(-z))
		if r.Float64() < p {
			y[i] = 1
		}
	}
	return X, y
}

func TestLogisticLearns(t *testing.T) {
	X, y := makeBinary(4000, 1)
	cfg := DefaultTrainConfig()
	cfg.Epochs = 200
	res := TrainLocal(NewLogisticRegression("t"), X, y, cfg)
	first := res.LossHistory[0]
	last := res.LossHistory[len(res.LossHistory)-1]
	if last >= first {
		t.Fatalf("la log-loss no bajó: %.4f -> %.4f", first, last)
	}
	m := EvalClassification(NewLogisticRegression("t"), X, y, res.Weights)
	if m.AUC < 0.8 {
		t.Errorf("AUC demasiado baja: %.3f", m.AUC)
	}
	t.Logf("logística: loss %.4f->%.4f, AUC=%.3f, acc=%.3f", first, last, m.AUC, m.Accuracy)
}

func TestLinearLearns(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	n := 3000
	X := make([][]float64, n)
	y := make([]float64, n)
	for i := 0; i < n; i++ {
		a, b := r.NormFloat64(), r.NormFloat64()
		X[i] = []float64{a, b}
		y[i] = 3.0*a - 1.5*b + 0.7 + r.NormFloat64()*0.05
	}
	cfg := DefaultTrainConfig()
	cfg.Epochs = 300
	res := TrainLocal(NewLinearRegression("t"), X, y, cfg)
	preds := make([]float64, n)
	lm := NewLinearRegression("t")
	for i := range X {
		preds[i] = lm.Predict(X[i], res.Weights)
	}
	m := EvalRegression(preds, y)
	if m.R2 < 0.98 {
		t.Errorf("R² demasiado bajo: %.4f", m.R2)
	}
	t.Logf("lineal: R²=%.4f, RMSE=%.4f, w=%v", m.R2, m.RMSE, res.Weights)
}

// TestDistributedEquivalence es la prueba clave de correctitud del
// entrenamiento distribuido: el gradiente calculado de forma centralizada
// sobre TODO el dataset debe ser idéntico (hasta error de punto flotante)
// a la SUMA de los gradientes parciales calculados sobre shards disjuntos.
// Esto garantiza que repartir los datos entre workers y agregar por
// map-reduce produce exactamente el mismo modelo que entrenar monolítico.
func TestDistributedEquivalence(t *testing.T) {
	X, y := makeBinary(5000, 7)
	m := NewLogisticRegression("t")
	w := make([]float64, len(X[0])+1)
	for i := range w {
		w[i] = 0.1 * float64(i+1) // pesos arbitrarios no triviales
	}

	central := RawGradientConcurrent(m, X, y, w, 4)

	// Partir en 3 shards disjuntos (tamaños desiguales a propósito).
	bounds := []int{0, 1234, 3000, len(X)}
	agg := NewGrad(len(w))
	for s := 0; s < 3; s++ {
		lo, hi := bounds[s], bounds[s+1]
		sh := &ShardState{X: X[lo:hi], YMort: y[lo:hi]}
		agg.Add(sh.PartialGradient(KindMortality, w, 2))
	}

	if agg.N != central.N {
		t.Fatalf("N distinto: central=%d agregado=%d", central.N, agg.N)
	}
	for i := range central.Sum {
		if math.Abs(central.Sum[i]-agg.Sum[i]) > 1e-9 {
			t.Fatalf("gradiente difiere en w[%d]: central=%.12f agregado=%.12f", i, central.Sum[i], agg.Sum[i])
		}
	}
	if math.Abs(central.Loss-agg.Loss) > 1e-9 {
		t.Fatalf("pérdida difiere: central=%.12f agregado=%.12f", central.Loss, agg.Loss)
	}
	t.Logf("equivalencia OK: gradiente centralizado == Σ gradientes de 3 shards (N=%d)", central.N)
}

func TestScalerStandardizes(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	n, d := 2000, 4
	X := make([][]float64, n)
	for i := range X {
		row := make([]float64, d)
		for j := range row {
			row[j] = 10*float64(j) + r.NormFloat64()*float64(j+1)
		}
		X[i] = row
	}
	sc := FitScaler(X)
	Xs := sc.TransformMatrix(X)
	// media ~0, desv ~1 por columna
	for j := 0; j < d; j++ {
		var mean float64
		for i := range Xs {
			mean += Xs[i][j]
		}
		mean /= float64(n)
		if math.Abs(mean) > 1e-6 {
			t.Errorf("columna %d media no es ~0: %.6f", j, mean)
		}
	}
}
