#!/usr/bin/env bash
# scripts/setup_synthea.sh
# Descarga el JAR de Synthea (sin clonar el repositorio) y lo ejecuta para el
# modulo "prostate_cancer". La salida cruda en CSV se deposita en
# data/synthea_raw/.
#
# Variables de entorno opcionales:
#   SYNTHEA_POP   tamano de poblacion base (default 10000)
#   SYNTHEA_SEED  semilla para reproducibilidad (default 42)
#
# Requisitos: Java 11+ en PATH, curl o wget.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
JAR_DIR="$PROJECT_ROOT/data/synthea_jar"
OUT_DIR="$PROJECT_ROOT/data/synthea_raw"
JAR_PATH="$JAR_DIR/synthea-with-dependencies.jar"
URL="https://github.com/synthetichealth/synthea/releases/latest/download/synthea-with-dependencies.jar"

POP="${SYNTHEA_POP:-10000}"
SEED="${SYNTHEA_SEED:-42}"

mkdir -p "$JAR_DIR" "$OUT_DIR"

if [ ! -f "$JAR_PATH" ]; then
    echo "[setup_synthea] descargando JAR desde $URL ..."
    if command -v curl >/dev/null 2>&1; then
        curl -L --fail -o "$JAR_PATH" "$URL"
    elif command -v wget >/dev/null 2>&1; then
        wget -O "$JAR_PATH" "$URL"
    else
        echo "[setup_synthea] se requiere curl o wget para descargar el JAR" >&2
        exit 1
    fi
else
    echo "[setup_synthea] JAR ya presente en $JAR_PATH (omitiendo descarga)"
fi

if ! command -v java >/dev/null 2>&1; then
    echo "[setup_synthea] Java no esta en PATH. Instalar JDK/JRE 11+." >&2
    exit 1
fi

echo "[setup_synthea] ejecutando Synthea: -p $POP -g M -m prostate_cancer ..."
java -jar "$JAR_PATH" \
    -p "$POP" \
    -s "$SEED" \
    -g M \
    -m "prostate_cancer" \
    --exporter.csv.export=true \
    --exporter.fhir.export=false \
    --exporter.ccda.export=false \
    --exporter.text.export=false \
    --exporter.baseDirectory="$OUT_DIR"

echo ""
echo "[setup_synthea] OK. CSVs crudos en: $OUT_DIR/csv"
echo "[setup_synthea] siguiente paso: python scripts/generate_<dataset>.py"
