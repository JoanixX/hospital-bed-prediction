# scripts/ — generación de datos simulados vía Synthea

Cada dataset se genera con un script independiente (uno por archivo CSV),
basados en el output crudo del proyecto open-source [Synthea](https://github.com/synthetichealth/synthea).
Cada archivo producido cumple con **al menos 1.5M registros** según el
requisito del informe.

## Requisitos

- **Java 11+**. Si no está instalado, `setup_synthea.ps1` descarga
  automáticamente un **JDK portable** (Eclipse Temurin 17) a `data/jdk/`
  y lo usa solo para esa corrida. No requiere admin ni modifica el PATH global.
- **Python 3.10+** con `pandas` y `numpy`:
  ```bash
  pip install pandas numpy
  ```
- **PowerShell 5.1+** (Windows) o **bash + curl/wget** (Linux/Mac)

> No se clona el repositorio de Synthea. El script `setup_synthea` descarga
> directamente el JAR `synthea-with-dependencies.jar` desde la página de
> releases de GitHub. El JAR y los CSVs crudos están excluidos por `.gitignore`.

## Archivos del directorio

| Script                          | Descripción                                                              |
|---------------------------------|--------------------------------------------------------------------------|
| `setup_synthea.ps1`             | Descarga JDK portable (si Java no está en PATH), descarga el JAR de Synthea y ejecuta el módulo `prostate_cancer` (Windows) |
| `setup_synthea.sh`              | Equivalente bash para Linux/Mac                                          |
| `_common.py`                    | Utilidades compartidas: `load_raw`, `resize_to_target`, `write_csv`, constantes `TARGET_ROWS`/`SEED` |
| `generate_patients.py`          | Produce `data/patients.csv` con el esquema plano que consume `internal/loader` |
| `generate_encounters.py`        | Produce `data/encounters.csv`                                            |
| `generate_observations.py`      | Produce `data/observations.csv`                                          |
| `generate_claims.py`            | Produce `data/claims.csv`                                                |
| `generate_claims_transactions.py` | Produce `data/claims_transactions.csv`                                 |
| `eda.py`                        | Análisis exploratorio del dataset + gráficos PNG en `data/eda_plots/`   |
| `plot_results.py`               | Genera `bench_time.png`, `bench_speedup.png` y `bench_cpu.png` a partir de `data/bench_results.csv` |

## Flujo de generación

### 1. Generar la base cruda con Synthea (una sola vez)

Descarga el JAR y ejecuta Synthea con el módulo `prostate_cancer`. La
salida queda en `data/synthea_raw/csv/`.

```powershell
# Windows
powershell -ExecutionPolicy Bypass -File scripts/setup_synthea.ps1
```

```bash
# Linux / Mac / Git Bash
bash scripts/setup_synthea.sh
```

Variables de entorno opcionales:

| Variable        | Default  | Descripción                                       |
|-----------------|----------|---------------------------------------------------|
| `SYNTHEA_POP`   | `10000`  | Tamaño de la población base de Synthea            |
| `SYNTHEA_SEED`  | `42`     | Semilla determinista para Synthea y oversampling  |

Subir `SYNTHEA_POP` reduce la cantidad de oversampling posterior pero
aumenta el tiempo de generación (Synthea simula vidas completas).

### 2. Generar cada dataset oversampleado (≥1.5M filas c/u)

Cada script lee su CSV correspondiente de `data/synthea_raw/csv/`,
realiza bootstrap con reemplazo (o subsample si la base es mayor) hasta
exactamente `SYNTHEA_TARGET_ROWS` filas (default 1 500 000) y deposita
el resultado en `data/`.

```bash
python scripts/generate_patients.py            # data/patients.csv
python scripts/generate_encounters.py          # data/encounters.csv
python scripts/generate_observations.py        # data/observations.csv
python scripts/generate_claims.py              # data/claims.csv
python scripts/generate_claims_transactions.py # data/claims_transactions.csv
```

Los scripts son independientes y pueden ejecutarse en cualquier orden o
en paralelo en distintas terminales.

Cambiar el número de filas objetivo sin tocar el código:

```bash
SYNTHEA_TARGET_ROWS=2000000 python scripts/generate_patients.py
```

### 3. Análisis exploratorio

```bash
python scripts/eda.py --input data/patients.csv --output data/eda_plots
```

Genera los histogramas y mapas de calor de correlación usados en el informe.

### 4. Gráficos de benchmark

Después de ejecutar `go test -bench=. -benchmem -cpu=1,2,4,8 ./benchmarks/`
y anotar los tiempos en `data/bench_results.csv`:

```bash
python scripts/plot_results.py --input data/bench_results.csv --output data/eda_plots
```

Produce `bench_time.png`, `bench_speedup.png` y `bench_cpu.png`.

## Esquemas de los datasets generados

### `patients.csv`

Esquema plano consumible por `internal/loader/loader.go`:

| Columna           | Tipo    | Descripción                                             |
|-------------------|---------|---------------------------------------------------------|
| `id`              | string  | Identificador único del paciente                        |
| `age`             | int     | Edad en años (derivado de `BIRTHDATE`)                  |
| `race`            | string  | Raza (lowercase; de Synthea `RACE`)                     |
| `ethnicity`       | string  | Etnicidad (lowercase; de Synthea `ETHNICITY`)           |
| `marital`         | string  | Estado civil (lowercase; de Synthea `MARITAL`)          |
| `income`          | float64 | Ingresos anuales en USD (de Synthea `INCOME`)           |
| `coverage`        | float64 | Cobertura médica [0.0–1.0] (`HEALTHCARE_COVERAGE`)      |
| `healthcare_cost` | float64 | Gasto médico acumulado en USD (`HEALTHCARE_EXPENSES`)   |
| `psa`             | float64 | Antígeno Prostático Específico en ng/mL (de `observations.csv`; imputado con lognormal si falta) |
| `num_encounters`  | int     | Número de encuentros médicos (agregado de `encounters.csv`) |
| `num_diagnoses`   | int     | Número de diagnósticos (de `conditions.csv` o `claims.DIAGNOSIS*`) |
| `has_died`        | bool    | `true` si el paciente ha fallecido                      |
| `survival_days`   | int     | Días desde nacimiento hasta muerte o fecha de corte     |

### Otros datasets

| Dataset                   | Descripción                                                        |
|---------------------------|--------------------------------------------------------------------|
| `encounters.csv`          | Columnas nativas de Synthea. `Id` regenerado para PK única.        |
| `observations.csv`        | Columnas nativas de Synthea, sin clave primaria propia.            |
| `claims.csv`              | Columnas nativas de Synthea. `Id` regenerado.                      |
| `claims_transactions.csv` | Columnas nativas de Synthea. `ID` regenerado.                      |

## Lógica de oversampling (`_common.py`)

`resize_to_target` garantiza exactamente `TARGET_ROWS` filas:

- `len(df) > target` → subsample sin reemplazo (preserva distribución, sin duplicados)
- `len(df) < target` → bootstrap con reemplazo (mantiene distribuciones marginales)
- `len(df) == target` → copia directa

Cuando se hace bootstrap, las columnas indicadas en `id_columns` se regeneran
con IDs secuenciales únicos (`{id_prefix}-{n:09d}`) para mantener la propiedad
de clave primaria.

## Limitaciones conocidas

- El bootstrap mantiene las distribuciones marginales pero **rompe la
  integridad referencial entre tablas**: la columna `PATIENT` en encounters
  no corresponde necesariamente a un `id` real del `patients.csv`
  oversampleado. Cada dataset es estadísticamente representativo de forma
  independiente, que es lo requerido por el pipeline concurrente.
- El esquema de `patients.csv` se mantiene plano (sin normalización) para
  ser compatible con `internal/loader/loader.go`. La transformación de
  nombres de columnas Synthea (`Id`, `BIRTHDATE`, `RACE`, etc.) a los del
  loader se realiza dentro de `generate_patients.py`.
- El campo `psa` se imputa con una distribución lognormal (`mean=1.5, sigma=0.6`)
  cuando no existe observación de "Prostate specific Ag" para el paciente.
