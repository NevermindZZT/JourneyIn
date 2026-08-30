param(
  [string]$ServerUrl = "http://127.0.0.1:8080",
  [string]$WebUrl = "http://127.0.0.1:8080"
)

$ErrorActionPreference = "Stop"
$ServerUrl = $ServerUrl.TrimEnd('/')
$WebUrl = $WebUrl.TrimEnd('/')

function Assert-Ok([bool]$Condition, [string]$Message) {
  if (-not $Condition) { throw $Message }
}

$health = Invoke-RestMethod -Uri "$ServerUrl/api/v1/health"
Assert-Ok ($health.status -eq "ok") "health check failed"

$capabilities = Invoke-RestMethod -Uri "$ServerUrl/api/v1/capabilities"
Assert-Ok ($capabilities.schema_versions -contains 1) "Trip schema v1 is not advertised"
Assert-Ok ($null -ne $capabilities.mcp) "MCP capability is missing"
Assert-Ok ($null -ne $capabilities.map_providers.amap -and $capabilities.map_providers.amap.registered) "AMap provider capability is missing"
Assert-Ok ($capabilities.default_map_provider -in @('baidu', 'amap')) "Default map provider capability is invalid"
$settings = Invoke-RestMethod -Uri "$ServerUrl/api/v1/settings"
Assert-Ok ($settings.map.default_provider -in @('baidu', 'amap')) "Default map provider setting is invalid"

$schema = Invoke-WebRequest -UseBasicParsing -Uri "$ServerUrl/api/v1/schema/trip/v1.json"
Assert-Ok ($schema.StatusCode -eq 200) "Trip schema endpoint failed"
Assert-Ok ($schema.Content.Length -gt 500) "Trip schema is unexpectedly small"

$web = Invoke-WebRequest -UseBasicParsing -Uri "$WebUrl/"
Assert-Ok ($web.StatusCode -eq 200) "Web root failed"
Assert-Ok ($web.Content.Contains("JourneyIn")) "Web root does not contain JourneyIn"
$amapSmoke = Invoke-WebRequest -UseBasicParsing -Uri "$WebUrl/amap-smoke.html"
Assert-Ok ($amapSmoke.StatusCode -eq 200 -and $amapSmoke.Content.Contains("AMap JS API 2.0")) "AMap smoke page failed"

Write-Host "Local verification passed"
Write-Host ("server=" + $ServerUrl)
Write-Host ("web=" + $WebUrl)
Write-Host ("version=" + $health.version)
Write-Host ("schema_bytes=" + $schema.Content.Length)
