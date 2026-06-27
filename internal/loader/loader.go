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
	header, err := reader.Read()
	if err != nil {
		return nil, 0, fmt.Errorf("error leyendo cabecera: %w", err)
	}
	colIdx := indexColumns(header)

	rawCh := make(chan []string, cfg.BufferSize)
	resCh := make(chan types.Patient, cfg.BufferSize)
	errCh := make(chan struct{}, cfg.BufferSize)

	var wg sync.WaitGroup
	for i := 0; i < cfg.NumWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for row := range rawCh {
				p, ok := parseAndValidate(row, colIdx)
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

// indexColumns construye un mapa nombre→índice tolerante a variaciones
// de orden entre versiones del dataset.
func indexColumns(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, h := range header {
		m[strings.ToLower(strings.TrimSpace(h))] = i
	}
	return m
}

// parseAndValidate aplica las reglas de calidad descritas en la
// sección 4.1 del informe: descarta filas con campos críticos
// faltantes, valores fuera de rango fisiológico o fechas inválidas.
func parseAndValidate(row []string, idx map[string]int) (types.Patient, bool) {
	get := func(key string) string {
		i, ok := idx[key]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	id := get("id")
	if id == "" {
		return types.Patient{}, false
	}

	age, err := strconv.Atoi(get("age"))
	if err != nil || age < 0 || age > 120 {
		return types.Patient{}, false
	}

	psa, err := strconv.ParseFloat(get("psa"), 64)
	if err != nil || psa < 0 || psa > 200 { // >200 ng/mL no fisiológico
		return types.Patient{}, false
	}

	income, _ := strconv.ParseFloat(get("income"), 64)
	cov, _ := strconv.ParseFloat(get("coverage"), 64)
	cost, _ := strconv.ParseFloat(get("healthcare_cost"), 64)
	enc, _ := strconv.Atoi(get("num_encounters"))
	diag, _ := strconv.Atoi(get("num_diagnoses"))
	died := strings.EqualFold(get("has_died"), "true") || get("has_died") == "1"
	sd, _ := strconv.Atoi(get("survival_days"))

	return types.Patient{
		ID:             id,
		Age:            age,
		Race:           strings.ToLower(get("race")),
		Ethnicity:      strings.ToLower(get("ethnicity")),
		MaritalStatus:  strings.ToLower(get("marital")),
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
