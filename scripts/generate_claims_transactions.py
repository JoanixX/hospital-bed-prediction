"""generate_claims_transactions.py
Construye data/claims_transactions.csv a partir del output crudo de Synthea
(data/synthea_raw/csv/claims_transactions.csv) y lo escala a >= 1.5M filas.

Esquema Synthea claims_transactions (resumido):
    ID, CLAIMID, CHARGEID, PATIENTID, TYPE, AMOUNT, METHOD, FROMDATE,
    TODATE, PLACEOFSERVICE, PROCEDURECODE, MODIFIER1..2, DIAGNOSISREF1..4,
    UNITS, DEPARTMENTID, NOTES, UNITAMOUNT, TRANSFEROUTID, TRANSFERTYPE,
    PAYMENTS, ADJUSTMENTS, TRANSFERS, OUTSTANDING, APPOINTMENTID,
    LINENOTE, PATIENTINSURANCEID, FEESCHEDULEID, PROVIDERID, SUPERVISINGPROVIDERID

Ejecucion:
    python scripts/generate_claims_transactions.py
"""
from __future__ import annotations

from _common import TARGET_ROWS, load_raw, resize_to_target, write_csv


def main() -> None:
    print("[claims_transactions] cargando claims_transactions crudos de Synthea...")
    df = load_raw("claims_transactions")

    # Synthea usa "ID" (mayuscula) en claims_transactions, no "Id". Regeneramos
    # la columna que realmente exista para mantener unicidad de PK.
    id_col = "ID" if "ID" in df.columns else ("Id" if "Id" in df.columns else None)
    id_columns = [id_col] if id_col else None

    print(f"[claims_transactions] base Synthea: {len(df):,} filas")
    df = resize_to_target(df, TARGET_ROWS, id_columns=id_columns, id_prefix="CTX")
    write_csv(df, "claims_transactions")


if __name__ == "__main__":
    main()
