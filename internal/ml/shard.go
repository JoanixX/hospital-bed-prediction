package ml

// Este archivo soporta el ENTRENAMIENTO DISTRIBUIDO. El master ajusta el
// escalador global y reparte un shard (porción del train set ya
// estandarizado) a cada worker. En cada época el master difunde los pesos
// actuales y cada worker devuelve el gradiente PARCIAL (suma sin promediar)
// sobre su shard, calculado con goroutines. El master agrega todos los
// parciales (map-reduce), promedia, aplica L2 y actualiza los pesos.

// ModelKind identifica cuál de los tres modelos se está entrenando/usando
// en una llamada RPC.
type ModelKind int

const (
	KindMortality ModelKind = iota
	KindSurvival
	KindCost
)

func (k ModelKind) Model() Model {
	switch k {
	case KindMortality:
		return mortModel
	case KindSurvival:
		return survModel
	default:
		return costModel
	}
}

func (k ModelKind) String() string {
	switch k {
	case KindMortality:
		return "mortalidad"
	case KindSurvival:
		return "supervivencia"
	default:
		return "costo"
	}
}

// ShardState es el estado que cada worker mantiene en memoria: su porción
// de features estandarizadas y los tres objetivos (mortalidad sin escalar;
// supervivencia y costo ya estandarizados con la estadística global del
// master).
type ShardState struct {
	X     [][]float64
	YMort []float64
	YSurv []float64 // estandarizado
	YCost []float64 // estandarizado
}

// PartialGradient calcula la suma de gradientes (Grad sin promediar) del
// modelo kind sobre el shard local, usando numLocalWorkers goroutines.
// Es exactamente lo que el worker devuelve por RPC al master cada época.
func (s *ShardState) PartialGradient(kind ModelKind, weights []float64, numLocalWorkers int) Grad {
	var y []float64
	switch kind {
	case KindMortality:
		y = s.YMort
	case KindSurvival:
		y = s.YSurv
	default:
		y = s.YCost
	}
	return RawGradientConcurrent(kind.Model(), s.X, y, weights, numLocalWorkers)
}

// KindFromString mapea el identificador RPC ("mortality"/"survival"/"cost")
// a ModelKind.
func KindFromString(s string) ModelKind {
	switch s {
	case "survival":
		return KindSurvival
	case "cost":
		return KindCost
	default:
		return KindMortality
	}
}

// RPCName devuelve el identificador RPC del kind.
func (k ModelKind) RPCName() string {
	switch k {
	case KindSurvival:
		return "survival"
	case KindCost:
		return "cost"
	default:
		return "mortality"
	}
}
