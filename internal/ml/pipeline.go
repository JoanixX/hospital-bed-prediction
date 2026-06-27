package ml

import "github.com/JoanixX/hospital-bed-prediction/internal/types"

// TrainedModels empaqueta todo lo necesario para predecir: el escalador
// de features, los pesos de los tres modelos y los escaladores de los
// dos objetivos continuos. Es serializable y se difunde a los workers
// para la fase de inferencia.
type TrainedModels struct {
	Scaler      *Scaler
	WMortality  []float64
	WSurvival   []float64
	SurvTarget  *TargetScaler
	WCost       []float64
	CostTarget  *TargetScaler
}

var (
	mortModel = NewLogisticRegression("mortalidad")
	survModel = NewLinearRegression("supervivencia")
	costModel = NewLinearRegression("costo")
)

// MortalityModel/SurvivalModel/CostModel exponen las instancias para que
// el coordinador distribuido y los tests usen exactamente las mismas.
func MortalityModel() Model { return mortModel }
func SurvivalModel() Model  { return survModel }
func CostModel() Model      { return costModel }

// PredictPatient aplica los tres modelos entrenados a un paciente y
// devuelve el resultado en unidades reales.
func (tm *TrainedModels) PredictPatient(p types.Patient, workerID int) types.PatientResult {
	x := tm.Scaler.Transform(FeatureVector(p))
	surv := tm.SurvTarget.Inverse(survModel.Predict(x, tm.WSurvival))
	if surv < 0 {
		surv = 0
	}
	cost := tm.CostTarget.Inverse(costModel.Predict(x, tm.WCost))
	if cost < 0 {
		cost = 0
	}
	return types.PatientResult{
		PatientID:        p.ID,
		MortalityRisk:    mortModel.Predict(x, tm.WMortality),
		SurvivalEstimate: surv,
		TreatmentCost:    cost,
		WorkerID:         workerID,
	}
}

// TrainReport resume las métricas del entrenamiento para el informe.
type TrainReport struct {
	NumTrain      int
	NumTest       int
	Epochs        int
	Mortality     ClassMetrics
	Survival      RegMetrics
	Cost          RegMetrics
	MortLoss      []float64
	SurvLoss      []float64
	CostLoss      []float64
}

// TrainAll entrena los tres modelos localmente (un proceso, paralelo con
// goroutines) sobre patients, evaluando en un split de test. Devuelve los
// modelos listos para inferencia y un reporte de métricas.
func TrainAll(patients []types.Patient, cfg TrainConfig, testFrac float64) (*TrainedModels, TrainReport) {
	ds := BuildDataset(patients)

	// Escalador global de features ajustado sobre TODO el set (en el
	// camino distribuido lo ajusta el master antes de repartir shards).
	scaler := FitScaler(ds.X)
	Xs := scaler.TransformMatrix(ds.X)

	// Split reproducible. Usamos los mismos índices para los tres
	// objetivos barajando con la misma semilla.
	const seed = 42
	xtrM, ytrM, xteM, yteM := TrainTestSplit(Xs, ds.YMortality, testFrac, seed)
	_, ytrS, _, yteS := TrainTestSplit(Xs, ds.YSurvival, testFrac, seed)
	_, ytrC, _, yteC := TrainTestSplit(Xs, ds.YCost, testFrac, seed)

	// Estandarizar objetivos continuos con estadística del TRAIN.
	survTS := FitTargetScaler(ytrS)
	costTS := FitTargetScaler(ytrC)
	ytrSz := survTS.ForwardAll(ytrS)
	ytrCz := costTS.ForwardAll(ytrC)

	rMort := TrainLocal(mortModel, xtrM, ytrM, cfg)
	rSurv := TrainLocal(survModel, xtrM, ytrSz, cfg)
	rCost := TrainLocal(costModel, xtrM, ytrCz, cfg)

	tm := &TrainedModels{
		Scaler: scaler, WMortality: rMort.Weights,
		WSurvival: rSurv.Weights, SurvTarget: survTS,
		WCost: rCost.Weights, CostTarget: costTS,
	}

	// Métricas en test (regresión de-estandarizada a escala real).
	survPred := make([]float64, len(xteM))
	costPred := make([]float64, len(xteM))
	for i := range xteM {
		survPred[i] = survTS.Inverse(survModel.Predict(xteM[i], rSurv.Weights))
		costPred[i] = costTS.Inverse(costModel.Predict(xteM[i], rCost.Weights))
	}

	rep := TrainReport{
		NumTrain:  len(xtrM),
		NumTest:   len(xteM),
		Epochs:    cfg.Epochs,
		Mortality: EvalClassification(mortModel, xteM, yteM, rMort.Weights),
		Survival:  EvalRegression(survPred, yteS),
		Cost:      EvalRegression(costPred, yteC),
		MortLoss:  rMort.LossHistory,
		SurvLoss:  rSurv.LossHistory,
		CostLoss:  rCost.LossHistory,
	}
	return tm, rep
}

// ---- Conversión modelo <-> bundle serializable (RPC/persistencia) ----

// ToBundle serializa los modelos entrenados a types.ModelBundle.
func (tm *TrainedModels) ToBundle() types.ModelBundle {
	return types.ModelBundle{
		Scaler:     types.ScalerParams{Mean: tm.Scaler.Mean, Std: tm.Scaler.Std},
		WMortality: tm.WMortality,
		WSurvival:  tm.WSurvival,
		WCost:      tm.WCost,
		SurvMean:   tm.SurvTarget.Mean,
		SurvStd:    tm.SurvTarget.Std,
		CostMean:   tm.CostTarget.Mean,
		CostStd:    tm.CostTarget.Std,
	}
}

// TrainedFromBundle reconstruye los modelos a partir de un bundle.
func TrainedFromBundle(b types.ModelBundle) *TrainedModels {
	return &TrainedModels{
		Scaler:     &Scaler{Mean: b.Scaler.Mean, Std: b.Scaler.Std},
		WMortality: b.WMortality,
		WSurvival:  b.WSurvival,
		SurvTarget: &TargetScaler{Mean: b.SurvMean, Std: b.SurvStd},
		WCost:      b.WCost,
		CostTarget: &TargetScaler{Mean: b.CostMean, Std: b.CostStd},
	}
}

// ScalerParamsFrom y ScalerFromParams convierten el estandarizador.
func ScalerParamsFrom(s *Scaler) types.ScalerParams {
	return types.ScalerParams{Mean: s.Mean, Std: s.Std}
}

func ScalerFromParams(p types.ScalerParams) *Scaler {
	return &Scaler{Mean: p.Mean, Std: p.Std}
}
