"""generate_encounters.py
Construye data/encounters.csv a partir del output crudo de Synthea
(data/synthea_raw/csv/encounters.csv) y lo escala a >= 1.5M filas mediante
bootstrap. Las columnas son las nativas de Synthea, sin transformacion.

Esquema Synthea encounters:
    Id, START, STOP, PATIENT, ORGANIZATION, PROVIDER, PAYER,
    ENCOUNTERCLASS, CODE, DESCRIPTION, BASE_ENCOUNTER_COST,
    TOTAL_CLAIM_COST, PAYER_COVERAGE, REASONCODE, REASONDESCRIPTION

Nota: al hacer bootstrap se regenera la columna Id (clave primaria) para
mantenerla unica. La columna PATIENT conserva su referencia al patient_id
original de Synthea; no se realinea con el patients.csv oversampleado
porque cada dataset se procesa de forma independiente segun el requisito
del proyecto.

Ejecucion:
    python scripts/generate_encounters.py
"""
from __future__ import annotations

from _common import TARGET_ROWS, load_raw, resize_to_target, write_csv


def main() -> None:
    print("[encounters] cargando encounters crudos de Synthea...")
    df = load_raw("encounters")
    print(f"[encounters] base Synthea: {len(df):,} filas")
    df = resize_to_target(df, TARGET_ROWS, id_columns=["Id"], id_prefix="ENC")
    write_csv(df, "encounters")


if __name__ == "__main__":
    main()
