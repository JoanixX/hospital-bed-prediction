# scripts/setup_synthea.ps1
# Descarga el JAR de Synthea (sin clonar el repositorio para no interferir con
# el repositorio git del proyecto) y lo ejecuta para el modulo
# "prostate_cancer". La salida cruda en CSV se deposita en data/synthea_raw/.
#
# Si Java no esta en PATH, descarga un JDK portable (Eclipse Temurin 17) a
# data/jdk/ y lo usa solo para esta ejecucion. NO requiere permisos de
# administrador, NO modifica el PATH global del sistema.
#
# Variables de entorno opcionales:
#   $env:SYNTHEA_POP   tamano de poblacion base (default 10000)
#   $env:SYNTHEA_SEED  semilla para reproducibilidad (default 42)

$ErrorActionPreference = "Stop"

$scriptDir   = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectRoot = Resolve-Path (Join-Path $scriptDir "..")
$jarDir      = Join-Path $projectRoot "data\synthea_jar"
$jdkDir      = Join-Path $projectRoot "data\jdk"
$outDir      = Join-Path $projectRoot "data\synthea_raw"
$jarPath     = Join-Path $jarDir "synthea-with-dependencies.jar"

$syntheaUrl  = "https://github.com/synthetichealth/synthea/releases/latest/download/synthea-with-dependencies.jar"
$jdkApiUrl   = "https://api.adoptium.net/v3/binary/latest/17/ga/windows/x64/jdk/hotspot/normal/eclipse"
$population  = if ($env:SYNTHEA_POP)  { $env:SYNTHEA_POP }  else { "10000" }
$seed        = if ($env:SYNTHEA_SEED) { $env:SYNTHEA_SEED } else { "42" }

New-Item -ItemType Directory -Force -Path $jarDir | Out-Null
New-Item -ItemType Directory -Force -Path $jdkDir | Out-Null
New-Item -ItemType Directory -Force -Path $outDir | Out-Null

# TLS 1.2 (necesario en Windows PowerShell 5.1 para descargar de GitHub/Adoptium)
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

# ---------------------------------------------------------------------------
# Paso 1: localizar (o descargar) Java
# ---------------------------------------------------------------------------
function Find-PortableJava {
    param([string]$Root)
    if (-not (Test-Path $Root)) { return $null }
    $candidate = Get-ChildItem -Path $Root -Recurse -Filter "java.exe" -ErrorAction SilentlyContinue |
                 Where-Object { $_.FullName -match "\\bin\\java\.exe$" } |
                 Select-Object -First 1
    if ($candidate) { return $candidate.FullName }
    return $null
}

$javaExe = $null
$javaCmd = Get-Command java -ErrorAction SilentlyContinue
if ($javaCmd) {
    $javaExe = $javaCmd.Source
    Write-Host "[setup_synthea] Java encontrado en PATH: $javaExe"
} else {
    $javaExe = Find-PortableJava -Root $jdkDir
    if ($javaExe) {
        Write-Host "[setup_synthea] usando JDK portable previamente descargado: $javaExe"
    } else {
        Write-Host "[setup_synthea] Java no esta en PATH. Descargando JDK portable Temurin 17 ..."
        Write-Host "[setup_synthea]   (sin admin, sin instalar; se extrae en data/jdk/)"
        $jdkZip = Join-Path $jdkDir "temurin17.zip"
        try {
            $ProgressPreference = 'SilentlyContinue'
            Invoke-WebRequest -Uri $jdkApiUrl -OutFile $jdkZip -UseBasicParsing
        } catch {
            Write-Error "No se pudo descargar el JDK portable: $($_.Exception.Message)"
            exit 1
        }
        Write-Host "[setup_synthea] extrayendo JDK ..."
        try {
            Expand-Archive -Path $jdkZip -DestinationPath $jdkDir -Force
            Remove-Item $jdkZip -Force
        } catch {
            Write-Error "No se pudo extraer el ZIP del JDK: $($_.Exception.Message)"
            exit 1
        }
        $javaExe = Find-PortableJava -Root $jdkDir
        if (-not $javaExe) {
            Write-Error "No se encontro java.exe tras extraer el JDK en $jdkDir"
            exit 1
        }
        Write-Host "[setup_synthea] JDK portable listo en: $javaExe"
    }
}

# Sanity check
& $javaExe -version
if ($LASTEXITCODE -ne 0) {
    Write-Error "El binario java seleccionado no se ejecuta correctamente."
    exit 1
}

# ---------------------------------------------------------------------------
# Paso 2: descargar el JAR de Synthea
# ---------------------------------------------------------------------------
if (-not (Test-Path $jarPath)) {
    Write-Host "[setup_synthea] descargando JAR de Synthea ..."
    try {
        $ProgressPreference = 'SilentlyContinue'
        Invoke-WebRequest -Uri $syntheaUrl -OutFile $jarPath -UseBasicParsing
    } catch {
        Write-Error "No se pudo descargar el JAR de Synthea: $($_.Exception.Message)"
        exit 1
    }
} else {
    Write-Host "[setup_synthea] JAR de Synthea ya presente en $jarPath (omitiendo descarga)"
}

# ---------------------------------------------------------------------------
# Paso 3: ejecutar Synthea
# ---------------------------------------------------------------------------
Write-Host "[setup_synthea] ejecutando Synthea: -p $population -g M -m prostate_cancer ..."
& $javaExe -jar $jarPath `
    -p $population `
    -s $seed `
    -g M `
    -m "prostate_cancer" `
    --exporter.csv.export=true `
    --exporter.fhir.export=false `
    --exporter.ccda.export=false `
    --exporter.text.export=false `
    --exporter.baseDirectory="$outDir"

if ($LASTEXITCODE -ne 0) {
    Write-Error "Synthea termino con codigo $LASTEXITCODE"
    exit $LASTEXITCODE
}

Write-Host ""
Write-Host "[setup_synthea] OK. CSVs crudos en: $outDir\csv"
Write-Host "[setup_synthea] siguiente paso: python scripts/generate_<dataset>.py"
