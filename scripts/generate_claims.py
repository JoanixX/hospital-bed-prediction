"""generate_claims.py
Construye data/claims.csv a partir del output crudo de Synthea
(data/synthea_raw/csv/claims.csv) y lo escala a >= 1.5M filas.

Esquema Synthea claims (resumido):
    Id, PATIENTID, PROVIDERID, PRIMARYPATIENTINSURANCEID,
    SECONDARYPATIENTINSURANCEID, DEPARTMENTID, PATIENTDEPARTMENTID,
    DIAGNOSIS1..8, REFERRINGPROVIDERID, APPOINTMENTID, CURRENTILLNESSDATE,
    SERVICEDATE, SUPERVISINGPROVIDERID, STATUS1..2, STATUSP, OUTSTANDING1..2,
    OUTSTANDINGP, LASTBILLEDDATE1..2, LASTBILLEDDATEP, HEALTHCARECLAIMTYPEID1..2

Ejecucion:
    python scripts/generate_claims.py
"""
from __future__ import annotations

from _common import TARGET_ROWS, load_raw, resize_to_target, write_csv


def main() -> None:
    print("[claims] cargando claims crudos de Synthea...")
    df = load_raw("claims")
    print(f"[claims] base Synthea: {len(df):,} filas")
    df = resize_to_target(df, TARGET_ROWS, id_columns=["Id"], id_prefix="CLM")
    write_csv(df, "claims")


if __name__ == "__main__":
    main()
