# install.ps1 — instala o Whitelist Proxy como Windows Service.
#
# Uso (PowerShell elevado / "Executar como Administrador"):
#
#     PS> .\install.ps1
#     PS> .\install.ps1 -BinaryPath .\proxy.exe -WhitelistPath .\whitelist.json
#
# Parâmetros:
#   -InstallDir    Pasta de instalação (default: C:\Program Files\WhitelistProxy)
#   -BinaryPath    Caminho do proxy.exe a copiar (default: .\proxy.exe)
#   -WhitelistPath Caminho do whitelist.json a copiar (opcional)
#   -StartNow      Inicia o serviço logo após instalar (default: $true)

[CmdletBinding()]
param(
    [string] $InstallDir    = 'C:\Program Files\WhitelistProxy',
    [string] $BinaryPath    = '.\proxy.exe',
    [string] $WhitelistPath = '.\whitelist.json',
    [bool]   $StartNow      = $true
)

$ErrorActionPreference = 'Stop'

function Assert-Admin {
    $current = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($current)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw 'Este script precisa ser executado como Administrador.'
    }
}

function Ensure-Dir($path) {
    if (-not (Test-Path -LiteralPath $path)) {
        New-Item -ItemType Directory -Path $path | Out-Null
    }
}

Assert-Admin

if (-not (Test-Path -LiteralPath $BinaryPath)) {
    throw "Binário não encontrado: $BinaryPath"
}

Write-Host "Instalando em $InstallDir"
Ensure-Dir $InstallDir
Ensure-Dir (Join-Path $InstallDir 'logs')

$dstBinary = Join-Path $InstallDir 'proxy.exe'
Copy-Item -LiteralPath $BinaryPath -Destination $dstBinary -Force
Write-Host "  proxy.exe copiado"

$dstWhitelist = Join-Path $InstallDir 'whitelist.json'
if (-not (Test-Path -LiteralPath $dstWhitelist)) {
    if (Test-Path -LiteralPath $WhitelistPath) {
        Copy-Item -LiteralPath $WhitelistPath -Destination $dstWhitelist -Force
        Write-Host "  whitelist.json copiado"
    } else {
        # Cria whitelist mínima — o admin edita depois.
        @"
{
  "rules": [
    {"pattern": "example.com", "type": "exact", "note": "exemplo - troque pela sua lista"}
  ]
}
"@ | Set-Content -Path $dstWhitelist -Encoding UTF8
        Write-Host "  whitelist.json criada (mínima)"
    }
} else {
    Write-Host "  whitelist.json já existe — preservada"
}

# Verifica se o serviço já existe e remove (para reinstalar limpo).
$svc = Get-Service -Name 'WhitelistProxy' -ErrorAction SilentlyContinue
if ($svc) {
    Write-Host "Serviço já instalado — removendo antes de reinstalar"
    if ($svc.Status -eq 'Running') {
        & $dstBinary stop  | Out-Null
    }
    & $dstBinary uninstall | Out-Null
}

Write-Host "Registrando serviço Windows"
& $dstBinary install
if ($LASTEXITCODE -ne 0) {
    throw "Falha registrando o serviço (exit $LASTEXITCODE)"
}

if ($StartNow) {
    Write-Host "Iniciando serviço"
    & $dstBinary start
    if ($LASTEXITCODE -ne 0) {
        throw "Falha iniciando o serviço (exit $LASTEXITCODE)"
    }
}

Write-Host ""
Write-Host "Instalação concluída."
Write-Host "  Binário:    $dstBinary"
Write-Host "  Whitelist:  $dstWhitelist"
Write-Host "  Logs:       $(Join-Path $InstallDir 'logs')"
Write-Host "  Token admin:$(Join-Path $InstallDir 'admin.token')"
Write-Host ""
Write-Host "Para parar:    & '$dstBinary' stop"
Write-Host "Para remover:  & '$dstBinary' uninstall"
Write-Host "Status:        & '$dstBinary' status"
