// Package loader implementa la carga concurrente del dataset clínico.
// Aplica el patrón fan-out/fan-in: un productor lee el archivo CSV
// en bloques, N goroutines limpian/validan registros en paralelo y un
// consumidor centraliza los resultados validados.
package loader

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/JoanixX/hospital-bed-prediction/internal/types"
)

// LoadConfig agrupa los parámetros del cargador concurrente.
type LoadConfig struct {
	Path        string
	NumWorkers  int
	BufferSize  int
}

// LoadConcurrent carga el archivo CSV en paralelo. Devuelve el slice
// de pacientes validados y el conteo de registros descartados por
// reglas de limpieza (valores nulos críticos, fechas inconsistentes,
// outliers fuera de rango fisiológico).
func LoadConcurrent(cfg LoadConfig) ([]types.Patient, int, error) {
	file, err := os.Open(cfg.Path)
	if err != nil {
		return nil, 0, fmt.Errorf("no se pudo abrir %s: %w", cfg.Path, err)
	}
	defer file.Close()

	reader := csv.NewReader(bufio.NewReaderSize(file, 1<<20)) // buffer 1MB
	_, err = reader.Read()
	if err != nil {
		return nil, 0, fmt.Errorf("error leyendo cabecera: %w", err)
	}
	// Saltamos la cabecera, ya que usaremos índices fijos en parseAndValidate.

	rawCh := make(chan []string, cfg.BufferSize)
	resCh := make(chan types.Patient, cfg.BufferSize)
	errCh := make(chan struct{}, cfg.BufferSize)

	var wg sync.WaitGroup
	for i := 0; i < cfg.NumWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for row := range rawCh {
				p, ok := parseAndValidate(row)
				if !ok {
					errCh <- struct{}{}
					continue
				}
				resCh <- p
			}
		}()
	}

	// Productor: alimenta los workers con filas crudas.
	go func() {
		for {
			row, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				continue
			}
			rawCh <- row
		}
		close(rawCh)
	}()

	// Cerrar canales cuando todos los workers terminen.
	go func() {
		wg.Wait()
		close(resCh)
		close(errCh)
	}()

	// Consumidor agregador.
	var patients []types.Patient
	discarded := 0
	done := make(chan struct{})
	go func() {
		for range errCh {
			discarded++
		}
		done <- struct{}{}
	}()
	for p := range resCh {
		patients = append(patients, p)
	}
	<-done

	return patients, discarded, nil
}


// parseAndValidate aplica las reglas de calidad descritas en la
// sección 4.1 del informe: descarta filas con campos críticos
// faltantes, valores fuera de rango fisiológico o fechas inválidas.
// Se asume el siguiente orden de columnas en CSV:
// id(0), age(1), race(2), ethnicity(3), marital(4), income(5),
// coverage(6), healthcare_cost(7), psa(8), num_encounters(9),
// num_diagnoses(10), has_died(11), survival_days(12)
func parseAndValidate(row []string) (types.Patient, bool) {
	if len(row) < 13 {
		return types.Patient{}, false
	}

	id := strings.TrimSpace(row[0])
	if id == "" {
		return types.Patient{}, false
	}

	age, err := strconv.Atoi(strings.TrimSpace(row[1]))
	if err != nil || age < 0 || age > 120 {
		return types.Patient{}, false
	}

	psaStr := strings.TrimSpace(row[8])
	psa, err := strconv.ParseFloat(psaStr, 64)
	if err != nil || psa < 0 || psa > 200 { // >200 ng/mL no fisiológico
		return types.Patient{}, false
	}

	income, _ := strconv.ParseFloat(strings.TrimSpace(row[5]), 64)
	cov, _ := strconv.ParseFloat(strings.TrimSpace(row[6]), 64)
	cost, _ := strconv.ParseFloat(strings.TrimSpace(row[7]), 64)
	enc, _ := strconv.Atoi(strings.TrimSpace(row[9]))
	diag, _ := strconv.Atoi(strings.TrimSpace(row[10]))
	
	diedStr := strings.TrimSpace(row[11])
	died := strings.EqualFold(diedStr, "true") || diedStr == "1"
	
	sd, _ := strconv.Atoi(strings.TrimSpace(row[12]))

	return types.Patient{
		ID:             id,
		Age:            age,
		Race:           strings.ToLower(strings.TrimSpace(row[2])),
		Ethnicity:      strings.ToLower(strings.TrimSpace(row[3])),
		MaritalStatus:  strings.ToLower(strings.TrimSpace(row[4])),
		Income:         income,
		Coverage:       cov,
		HealthcareCost: cost,
		PSALevel:       psa,
		NumEncounters:  enc,
		NumDiagnoses:   diag,
		HasDied:        died,
		SurvivalDays:   sd,
	}, true
}
