package ml

import "math/rand"

// Este archivo expone utilidades que usa el coordinador (Master) del
// entrenamiento distribuido para trabajar con índices de fila, de modo que
// los tres objetivos (mortalidad, supervivencia, costo) se separen con el
// MISMO split reproducible y se puedan shardear de forma alineada.

// SplitIndices baraja [0,n) con una semilla fija y devuelve los índices de
// train y de test. Usar índices (en vez de copiar matrices) permite aplicar
// el mismo split a varios objetivos sin desalinearlos.
func SplitIndices(n int, testFrac float64, seed int64) (train, test []int) {
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	r := rand.New(rand.NewSource(seed))
	r.Shuffle(n, func(i, j int) { idx[i], idx[j] = idx[j], idx[i] })
	nTest := int(float64(n) * testFrac)
	test = append(test, idx[:nTest]...)
	train = append(train, idx[nTest:]...)
	return train, test
}

// Subset selecciona las filas idx de la matriz X y del objetivo y.
func Subset(X [][]float64, y []float64, idx []int) ([][]float64, []float64) {
	xs := make([][]float64, len(idx))
	ys := make([]float64, len(idx))
	for i, id := range idx {
		xs[i] = X[id]
		ys[i] = y[id]
	}
	return xs, ys
}

// GatherRows selecciona solo filas de una matriz (sin objetivo).
func GatherRows(X [][]float64, idx []int) [][]float64 {
	out := make([][]float64, len(idx))
	for i, id := range idx {
		out[i] = X[id]
	}
	return out
}

// GatherY selecciona solo valores de un vector objetivo.
func GatherY(y []float64, idx []int) []float64 {
	out := make([]float64, len(idx))
	for i, id := range idx {
		out[i] = y[id]
	}
	return out
}
