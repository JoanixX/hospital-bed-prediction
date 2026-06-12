"""generate_patients.py
Construye data/patients.csv a partir del output crudo de Synthea
(data/synthea_raw/csv/) y lo escala a >= 1.5M filas.

Esquema producido (compatible con internal/loader/loader.go):
    id, age, race, ethnicity, marital, income, coverage, healthcare_cost,
    psa, num_encounters, num_diagnoses, has_died, survival_days

Los datos demograficos/financieros vienen directamente de Synthea
(patients.csv). PSA se calcula a partir de observations.csv filtrando
"Prostate specific Ag"; num_encounters y num_diagnoses son agregaciones
sobre encounters.csv y conditions.csv. has_died y survival_days se derivan
de BIRTHDATE / DEATHDATE.

Ejecucion:
    python scripts/generate_patients.py
"""
from __future__ import annotations

from datetime import date

import numpy as np
import pandas as pd

from _common import SEED, TARGET_ROWS, load_raw, raw_path, resize_to_target, write_csv


def _diagnosis_counts(claims: pd.DataFrame) -> pd.Series:
    """Cuenta diagnosticos por paciente desde claims.DIAGNOSIS1..8 (no nulos)."""
    diag_cols = [c for c in claims.columns if c.upper().startswith("DIAGNOSIS")]
    pid_col = "PATIENTID" if "PATIENTID" in claims.columns else "PATIENT"
    if not diag_cols or pid_col not in claims.columns:
        return pd.Series(dtype=int, name="num_diagnoses")
    nonnull = claims[diag_cols].notna().sum(axis=1)
    return nonnull.groupby(claims[pid_col]).sum().rename("num_diagnoses")


def main() -> None:
    print("[patients] cargando patients/encounters/observations...")
    pats = load_raw("patients")
    encs = load_raw("encounters")
    obs = load_raw("observations")

    # conditions.csv es opcional: el modulo prostate_cancer no siempre lo emite.
    # El informe usa claims.DIAGNOSIS para el conteo, asi que ese es el fallback.
    if raw_path("conditions").exists():
        print("[patients] usando conditions.csv para num_diagnoses")
        conds = load_raw("conditions")
        diag_counts = conds.groupby("PATIENT").size().rename("num_diagnoses")
    elif raw_path("claims").exists():
        print("[patients] conditions.csv ausente; derivando num_diagnoses desde claims.DIAGNOSIS*")
        claims_raw = load_raw("claims")
        diag_counts = _diagnosis_counts(claims_raw)
    else:
        print("[patients] sin fuente de diagnosticos: num_diagnoses=0")
        diag_counts = pd.Series(dtype=int, name="num_diagnoses")

    today = pd.Timestamp(date.today())
    pats["BIRTHDATE"] = pd.to_datetime(pats["BIRTHDATE"], errors="coerce")
    pats["DEATHDATE"] = pd.to_datetime(pats["DEATHDATE"], errors="coerce")

    pats["age_calc"] = ((today - pats["BIRTHDATE"]).dt.days / 365.25)
    pats["has_died"] = pats["DEATHDATE"].notna()
    end_ref = pats["DEATHDATE"].fillna(today)
    pats["survival_days"] = (end_ref - pats["BIRTHDATE"]).dt.days.clip(lower=0)

    enc_counts = encs.groupby("PATIENT").size().rename("num_encounters")

    psa_mask = obs["DESCRIPTION"].astype(str).str.contains(
        "Prostate specific", case=False, na=False
    )
    psa_obs = obs[psa_mask].copy()
    psa_obs["VALUE_NUM"] = pd.to_numeric(psa_obs["VALUE"], errors="coerce")
    psa_mean = psa_obs.groupby("PATIENT")["VALUE_NUM"].mean().rename("psa")

    merged = (
        pats.merge(enc_counts, left_on="Id", right_index=True, how="left")
            .merge(diag_counts, left_on="Id", right_index=True, how="left")
            .merge(psa_mean, left_on="Id", right_index=True, how="left")
    )

    rng = np.random.default_rng(SEED)
    missing_psa = merged["psa"].isna()
    if missing_psa.any():
        merged.loc[missing_psa, "psa"] = rng.lognormal(
            mean=1.5, sigma=0.6, size=int(missing_psa.sum())
        )
    merged["psa"] = merged["psa"].clip(lower=0.1, upper=200.0)

    out = pd.DataFrame({
        "id": merged["Id"].astype(str),
        "age": merged["age_calc"].fillna(68).clip(0, 120).round().astype(int),
        "race": merged["RACE"].astype(str).str.lower().fillna("other"),
        "ethnicity": merged["ETHNICITY"].astype(str).str.lower().fillna("unknown"),
        "marital": merged["MARITAL"].astype(str).str.lower().fillna("s"),
        "income": pd.to_numeric(merged["INCOME"], errors="coerce").fillna(0.0).round(2),
        "coverage": pd.to_numeric(merged["HEALTHCARE_COVERAGE"], errors="coerce")
                      .fillna(0.0).round(4),
        "healthcare_cost": pd.to_numeric(merged["HEALTHCARE_EXPENSES"], errors="coerce")
                             .fillna(0.0).round(2),
        "psa": merged["psa"].round(2),
        "num_encounters": merged["num_encounters"].fillna(0).astype(int),
        "num_diagnoses": merged["num_diagnoses"].fillna(0).astype(int),
        "has_died": merged["has_died"].astype(bool),
        "survival_days": merged["survival_days"].fillna(0).clip(lower=0).astype(int),
    })

    print(f"[patients] base Synthea: {len(out):,} pacientes")
    out = resize_to_target(out, TARGET_ROWS, id_columns=["id"], id_prefix="PAT")
    write_csv(out, "patients")


if __name__ == "__main__":
    main()
