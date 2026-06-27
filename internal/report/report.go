// Package report genera el reporte agregado del pipeline en formato
// legible por consola y, opcionalmente, en JSON para consumo posterior
// por la API REST (PC4).
package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/JoanixX/hospital-bed-prediction/internal/ml"
	"github.com/JoanixX/hospital-bed-prediction/internal/types"
)

// Print imprime un reporte completo con métricas por worker y
// agregados globales de los tres modelos predictivos.
func Print(results []types.PatientResult, stats []types.WorkerStats) {
	sep := strings.Repeat("-", 70)
	fmt.Println()
	fmt.Println(sep)
	fmt.Println("                    REPORTE DE RESULTADOS")
	fmt.Println(sep)

	fmt.Println("\nMétricas por Worker:")
	for _, s := range stats {
		fmt.Printf("  Worker %d -> %d pacientes procesados en %v\n",
			s.WorkerID, s.PatientsHandled, s.ProcessingTime.Round(time.Millisecond))
	}

	var totalMortality, totalSurvival, totalCost float64
	highRisk := 0
	for _, r := range results {
		totalMortality += r.MortalityRisk
		totalSurvival += r.SurvivalEstimate
		totalCost += r.TreatmentCost
		if r.MortalityRisk >= 0.6 {
			highRisk++
		}
	}
	n := float64(len(results))
	if n == 0 {
		fmt.Println("\n[report] no hay resultados que reportar")
		return
	}

	fmt.Println("\nModelo 1 - Clasificación de Mortalidad:")
	fmt.Printf("  Riesgo promedio de muerte      : %.2f%%\n", (totalMortality/n)*100)
	fmt.Printf("  Pacientes alto riesgo (>=60%%) : %d / %d\n", highRisk, len(results))

	fmt.Println("\nModelo 2 - Análisis de Supervivencia:")
	fmt.Printf("  Supervivencia promedio estimada: %.0f días (%.1f años)\n",
		totalSurvival/n, (totalSurvival/n)/365)

	fmt.Println("\nModelo 3 - Predicción de Costo:")
	fmt.Printf("  Costo promedio de tratamiento  : $%.2f USD\n", totalCost/n)
	fmt.Printf("  Costo total proyectado         : $%.2f USD\n", totalCost)

	fmt.Println("\nMuestra de resultados individuales (primeros 5):")
	fmt.Printf("  %-12s %-8s %-12s %-14s %-14s\n",
		"PatientID", "Worker", "Mortalidad%", "Supervivencia", "Costo USD")
	fmt.Println("  " + strings.Repeat("-", 65))
	limit := 5
	if len(results) < limit {
		limit = len(results)
	}
	for i := 0; i < limit; i++ {
		r := results[i]
		fmt.Printf("  %-12s %-8d %-12.1f %-14.0f %-14.2f\n",
			r.PatientID, r.WorkerID, r.MortalityRisk*100,
			r.SurvivalEstimate, r.TreatmentCost)
	}
	fmt.Println(sep)
}

// PrintTraining imprime el reporte del entrenamiento distribuido: tamaño
// de los conjuntos, convergencia de la pérdida por época y métricas en test.
func PrintTraining(rep ml.TrainReport) {
	sep := strings.Repeat("=", 70)
	fmt.Println()
	fmt.Println(sep)
	fmt.Println("              REPORTE DE ENTRENAMIENTO DISTRIBUIDO")
	fmt.Println(sep)
	fmt.Printf("Muestras: %d entrenamiento | %d test    Épocas: %d\n",
		rep.NumTrain, rep.NumTest, rep.Epochs)

	printCurve := func(name string, h []float64) {
		if len(h) == 0 {
			return
		}
		step := len(h) / 5
		if step == 0 {
			step = 1
		}
		fmt.Printf("\n  Convergencia %s (pérdida por época):\n    ", name)
		for e := 0; e < len(h); e += step {
			fmt.Printf("ep%d=%.4f  ", e, h[e])
		}
		fmt.Printf("ep%d=%.4f\n", len(h)-1, h[len(h)-1])
	}
	printCurve("mortalidad (log-loss)", rep.MortLoss)
	printCurve("supervivencia (MSE)", rep.SurvLoss)
	printCurve("costo (MSE)", rep.CostLoss)

	fmt.Println("\n  Métricas en test:")
	fmt.Printf("   Mortalidad (logística)  : AUC=%.3f  Accuracy=%.3f  LogLoss=%.4f\n",
		rep.Mortality.AUC, rep.Mortality.Accuracy, rep.Mortality.LogLoss)
	fmt.Printf("   Supervivencia (lineal)  : R²=%.3f  RMSE=%.0f días  MAE=%.0f días\n",
		rep.Survival.R2, rep.Survival.RMSE, rep.Survival.MAE)
	fmt.Printf("   Costo (lineal)          : R²=%.3f  RMSE=$%.0f  MAE=$%.0f\n",
		rep.Cost.R2, rep.Cost.RMSE, rep.Cost.MAE)
	fmt.Println(sep)
}
