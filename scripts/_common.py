"""Utilidades compartidas por los scripts generate_*.py.

Cada generate_*.py es independiente y se encarga de UN solo dataset. Esta
modulo agrupa logica comun de I/O y oversampling para no duplicar codigo
entre scripts; no contiene la logica de negocio de ningun dataset concreto.
"""
from __future__ import annotations

import os
from pathlib import Path

import numpy as np
import pandas as pd

SCRIPT_DIR = Path(__file__).resolve().parent
PROJECT_ROOT = SCRIPT_DIR.parent
RAW_DIR = PROJECT_ROOT / "data" / "synthea_raw" / "csv"
OUT_DIR = PROJECT_ROOT / "data"

TARGET_ROWS = int(os.environ.get("SYNTHEA_TARGET_ROWS", "1500000"))
SEED = int(os.environ.get("SYNTHEA_SEED", "42"))


def raw_path(name: str) -> Path:
    return RAW_DIR / f"{name}.csv"


def load_raw(name: str) -> pd.DataFrame:
    p = raw_path(name)
    if not p.exists():
        raise FileNotFoundError(
            f"No existe {p}. Ejecuta scripts/setup_synthea.ps1 (o .sh) primero."
        )
    return pd.read_csv(p, low_memory=False)


def resize_to_target(
    df: pd.DataFrame,
    target: int = TARGET_ROWS,
    id_columns: list[str] | None = None,
    id_prefix: str = "SYN",
    seed: int = SEED,
) -> pd.DataFrame:
    """Devuelve un DataFrame con exactamente `target` filas.

    - Si len(df) > target: muestra sin reemplazo (subsample).
    - Si len(df) < target: bootstrap con reemplazo (oversample).
    - Si len(df) == target: copia tal cual.

    Si se pasan `id_columns`, esos campos se regeneran como cadenas unicas
    para no romper la propiedad de clave primaria al hacer bootstrap.
    """
    n = len(df)
    if n == 0:
        raise ValueError("El dataframe origen esta vacio")

    rng = np.random.default_rng(seed)
    if n == target:
        out = df.reset_index(drop=True).copy()
    elif n > target:
        idx = rng.choice(n, size=target, replace=False)
        out = df.iloc[idx].reset_index(drop=True)
    else:
        idx = rng.integers(0, n, size=target)
        out = df.iloc[idx].reset_index(drop=True)

    if id_columns:
        width = max(9, len(str(target)))
        for col in id_columns:
            if col in out.columns:
                out[col] = [f"{id_prefix}-{i:0{width}d}" for i in range(1, target + 1)]
    return out


def write_csv(df: pd.DataFrame, name: str) -> Path:
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    p = OUT_DIR / f"{name}.csv"
    df.to_csv(p, index=False)
    size_mb = p.stat().st_size / (1024 * 1024)
    print(f"[{name}] escrito {p} ({len(df):,} filas, {size_mb:.1f} MB)")
    return p
