# scripts/ — generacion de datos simulados via Synthea

Cada dataset se genera con un script independiente (uno por archivo CSV),
basados en el output crudo del proyecto open-source [Synthea](https://github.com/synthetichealth/synthea).
Cada archivo producido cumple con **al menos 1.5M registros** segun el
requisito del informe PC3.

## Requisitos

- **Java 11+**. Si no está instalado, `setup_synthea.ps1` descarga
  automáticamente un **JDK portable** (Eclipse Temurin 17) a `data/jdk/`
  y lo usa solo para esta corrida. No requiere admin ni modifica PATH.
- **Python 3.10+** con `pandas` y `numpy`
  ```bash
  pip install pandas numpy
  ```
- **PowerShell 5.1+** (Windows) o **bash + curl/wget** (Linux/Mac)

> No se clona el repositorio de Synthea. El script `setup_synthea` descarga
> directamente el JAR `synthea-with-dependencies.jar` desde la pagina de
> releases. El JAR y los CSVs crudos se ignoran en `.gitignore`.

## Flujo de generacion

### 1. Generar la base cruda con Synthea (una sola vez)

Descarga el JAR y ejecuta Synthea sobre el modulo `prostate_cancer`. La
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

| Variable        | Default | Descripcion                                  |
|-----------------|---------|----------------------------------------------|
| `SYNTHEA_POP`   | `10000` | Tamano de la poblacion base de Synthea       |
| `SYNTHEA_SEED`  | `42`    | Semilla determinista para Synthea            |

Subir `SYNTHEA_POP` reduce la cantidad de oversampling posterior pero
aumenta el tiempo de generacion (Synthea simula vidas completas).

### 2. Generar cada dataset oversampleado (>= 1.5M filas c/u)

Cada script lee su CSV correspondiente de `data/synthea_raw/csv/`,
realiza bootstrap/subsample hasta exactamente `SYNTHEA_TARGET_ROWS`
(default 1.5M) y deposita el resultado en `data/`.

```bash
python scripts/generate_patients.py            # data/patients.csv
python scripts/generate_encounters.py          # data/encounters.csv
python scripts/generate_observations.py        # data/observations.csv
python scripts/generate_claims.py              # data/claims.csv
python scripts/generate_claims_transactions.py # data/claims_transactions.csv
```

Los scripts son independientes y pueden ejecutarse en cualquier orden o
en paralelo en distintas terminales. Variable opcional:
`SYNTHEA_TARGET_ROWS=2000000 python scripts/generate_patients.py`

## Esquemas resultantes

| Dataset                  | Esquema                                                |
|--------------------------|--------------------------------------------------------|
| `patients.csv`           | Esquema plano consumible por `internal/loader`: `id, age, race, ethnicity, marital, income, coverage, healthcare_cost, psa, num_encounters, num_diagnoses, has_died, survival_days`. PSA y los conteos se obtienen joineando con encounters/observations/conditions. |
| `encounters.csv`         | Columnas nativas de Synthea. `Id` regenerado para PK unica. |
| `observations.csv`       | Columnas nativas de Synthea, sin clave primaria propia.   |
| `claims.csv`             | Columnas nativas de Synthea. `Id` regenerado.             |
| `claims_transactions.csv`| Columnas nativas de Synthea. `ID` regenerado.             |

## Limitaciones conocidas

- El bootstrap mantiene las distribuciones marginales pero rompe la
  integridad referencial entre tablas (la columna `PATIENT` en encounters
  ya no necesariamente corresponde a un `id` real del patients.csv
  oversampleado). Cada dataset es estadisticamente representativo de
  forma independiente, que es lo que requiere el pipeline concurrente
  del proyecto.
- El esquema de `patients.csv` se mantiene plano para no romper
  `internal/loader/loader.go`. Las columnas nativas de Synthea
  (`Id`, `BIRTHDATE`, `RACE`, etc.) se traducen a las del loader durante
  `generate_patients.py`.
