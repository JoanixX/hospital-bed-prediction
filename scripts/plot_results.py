"""plot_results.py
Genera los tres graficos de benchmark de la seccion 4.5 del informe a
partir de data/bench_results.csv:

    bench_time.png     tiempo total por configuracion
    bench_speedup.png  speedup observado vs ideal
    bench_cpu.png      uso promedio de CPU por configuracion

Uso:
    python scripts/plot_results.py
    python scripts/plot_results.py --input data/bench_results.csv \
                                   --output data/eda_plots
"""
from __future__ import annotations
import argparse
from pathlib import Path
import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import pandas as pd

SCRIPT_DIR = Path(__file__).resolve().parent
PROJECT_ROOT = SCRIPT_DIR.parent
DEFAULT_INPUT = PROJECT_ROOT / "data" / "bench_results.csv"
DEFAULT_OUTPUT = PROJECT_ROOT / "data" / "eda_plots"

def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--input", type=Path, default=DEFAULT_INPUT)
    p.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    return p.parse_args()

def plot_time(df: pd.DataFrame, out: Path) -> Path:
    fig, ax = plt.subplots(figsize=(7, 4.5))
    colors = ["#cf3a3a", "#2c8eb5", "#1e6091"]
    bars = ax.bar(df["label"], df["time_s"], color=colors[: len(df)])
    ax.set_ylabel("Tiempo de procesamiento (s)")
    ax.set_title("Tiempo total por configuracion")
    for b, v in zip(bars, df["time_s"]):
        ax.text(b.get_x() + b.get_width() / 2, v,
                f"{v:.3f}s", ha="center", va="bottom", fontsize=10)
    ax.grid(axis="y", alpha=0.3)
    fig.tight_layout()
    path = out / "bench_time.png"
    fig.savefig(path, dpi=150)
    plt.close(fig)
    return path

def plot_speedup(df: pd.DataFrame, out: Path) -> Path:
    base = df.loc[df["workers"] == df["workers"].min(), "time_s"].iloc[0]
    df = df.copy()
    df["speedup"] = base / df["time_s"]

    fig, ax = plt.subplots(figsize=(7, 4.5))
    ax.plot(df["workers"], df["speedup"], "o-", color="#1e6091",
            label="Speedup observado", linewidth=2, markersize=8)
    ax.plot(df["workers"], df["workers"], "--", color="#888",
            label="Speedup ideal (lineal)", linewidth=1.5)
    ax.set_xlabel("Numero de workers")
    ax.set_ylabel("Speedup (t_sec / t_obs)")
    ax.set_title("Escalabilidad del worker pool")
    ax.set_xticks(df["workers"])
    ax.grid(alpha=0.3)
    ax.legend()
    fig.tight_layout()
    path = out / "bench_speedup.png"
    fig.savefig(path, dpi=150)
    plt.close(fig)
    return path

def plot_cpu(df: pd.DataFrame, out: Path) -> Path:
    fig, ax = plt.subplots(figsize=(7, 4.5))
    colors = ["#cf3a3a", "#2c8eb5", "#1e6091"]
    bars = ax.bar(df["label"], df["cpu_pct"], color=colors[: len(df)])
    ax.set_ylabel("Uso promedio de CPU (%)")
    ax.set_title("Uso de CPU por configuracion")
    ax.set_ylim(0, 100)
    for b, v in zip(bars, df["cpu_pct"]):
        ax.text(b.get_x() + b.get_width() / 2, v,
                f"{v:.0f}%", ha="center", va="bottom", fontsize=10)
    ax.grid(axis="y", alpha=0.3)
    fig.tight_layout()
    path = out / "bench_cpu.png"
    fig.savefig(path, dpi=150)
    plt.close(fig)
    return path

def main() -> None:
    args = parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    df = pd.read_csv(args.input)
    df = df.sort_values("workers").reset_index(drop=True)

    for path in (plot_time(df, args.output),
                 plot_speedup(df, args.output),
                 plot_cpu(df, args.output)):
        print(f"[plot_results] generado {path}")

if __name__ == "__main__":
    main()