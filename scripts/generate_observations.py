"""generate_observations.py
Construye data/observations.csv a partir del output crudo de Synthea
(data/synthea_raw/csv/observations.csv) y lo redimensiona a 1.5M filas
(downsample sin reemplazo si Synthea genero mas, oversample con
reemplazo si genero menos).

Esquema Synthea observations:
    DATE, PATIENT, ENCOUNTER, CATEGORY, CODE, DESCRIPTION, VALUE,
    UNITS, TYPE

observations.csv en Synthea no tiene columna Id propia; la fila se
identifica por (DATE, PATIENT, ENCOUNTER, CODE). No se regenera ningun
identificador en el oversampling.

Ejecucion:
    python scripts/generate_observations.py
"""
from __future__ import annotations

from _common import TARGET_ROWS, load_raw, resize_to_target, write_csv


def main() -> None:
    print("[observations] cargando observations crudos de Synthea...")
    df = load_raw("observations")
    print(f"[observations] base Synthea: {len(df):,} filas")
    df = resize_to_target(df, TARGET_ROWS)
    write_csv(df, "observations")


if __name__ == "__main__":
    main()
