#!/usr/bin/env bash
# scripts/load_test.sh — Prueba de carga y latencia (Tarea 9)
#
# Requisitos:
#   go install github.com/rakyll/hey@latest
#   La API debe estar corriendo: go run ./cmd/api
#
# Uso:
#   bash scripts/load_test.sh            # usa defaults
#   API_ADDR=http://localhost:9090 bash scripts/load_test.sh

set -euo pipefail

API_ADDR="${API_ADDR:-http://localhost:8080}"
RESULTS_DIR="data/load_test_results"
mkdir -p "$RESULTS_DIR"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

echo "════════════════════════════════════════════════════"
echo "  Prueba de Carga — Hospital Bed Prediction API"
echo "  Objetivo: P99 < 100ms"
echo "  Target:   $API_ADDR"
echo "════════════════════════════════════════════════════"

# ── 1. Obtener token JWT ──────────────────────────────────────────────────────
echo ""
echo "[1/4] Obteniendo token JWT..."
TOKEN=$(curl -s -X POST "$API_ADDR/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"hospital2024"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")

if [ -z "$TOKEN" ]; then
  echo "ERROR: no se pudo obtener el token. ¿Está corriendo la API?"
  exit 1
fi
echo "Token obtenido: ${TOKEN:0:30}..."

# ── 2. Warmup: llenar la caché con el mismo payload ───────────────────────────
echo ""
echo "[2/4] Warmup (5 peticiones para pre-llenar Redis)..."
for i in $(seq 1 5); do
  curl -s -o /dev/null -X POST "$API_ADDR/predict" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    -d '{"id":"WARMUP","age":65,"race":"white","income":55000,"psa_level":8.5,"coverage":0.75,"num_encounters":6,"num_diagnoses":2}'
done
echo "Warmup completado."

# ── 3. Escenario A: peticiones concurrentes con caché activa ──────────────────
echo ""
echo "[3/4] Escenario A — 1000 req, 50 concurrentes (cache hit esperado)..."
hey -n 1000 -c 50 \
    -m POST \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    -d '{"id":"PAT-001","age":65,"race":"white","income":55000,"psa_level":8.5,"coverage":0.75,"num_encounters":6,"num_diagnoses":2}' \
    "$API_ADDR/predict" \
    | tee "$RESULTS_DIR/scenario_a_cache_${TIMESTAMP}.txt"

# ── 4. Escenario B: payloads variados (cache misses) ─────────────────────────
echo ""
echo "[4/4] Escenario B — 500 req, 20 concurrentes (payloads variados, cache miss)..."
hey -n 500 -c 20 \
    -m POST \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    -d '{"id":"PAT-VAR","age":72,"race":"black","income":32000,"psa_level":15.2,"coverage":0.40,"num_encounters":12,"num_diagnoses":4}' \
    "$API_ADDR/predict" \
    | tee "$RESULTS_DIR/scenario_b_nomatch_${TIMESTAMP}.txt"

# ── 5. Resumen de stats ───────────────────────────────────────────────────────
echo ""
echo "── Métricas del sistema post-test ──"
curl -s "$API_ADDR/stats" | python3 -m json.tool

echo ""
echo "Resultados guardados en: $RESULTS_DIR/"
echo ""
echo "Para JMeter (alternativa gráfica):"
echo "  1. Abrir JMeter → File > Open → scripts/jmeter_plan.jmx"
echo "  2. Configurar el token en HTTP Header Manager"
echo "  3. Run → ver Dashboard en tiempo real"
