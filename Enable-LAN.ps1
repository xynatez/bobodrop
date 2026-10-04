param([string]$Executable = (Join-Path $PSScriptRoot "bobodrop.exe"))
$ErrorActionPreference = "Stop"
$resolvedExecutable = (Resolve-Path -LiteralPath $Executable).Path
if ([IO.Path]::GetFileName($resolvedExecutable) -ne "bobodrop.exe") {
    throw "Pilih executable bobodrop.exe."
}
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    $arguments = '-NoProfile -ExecutionPolicy Bypass -File "' + $PSCommandPath + '" -Executable "' + $resolvedExecutable + '"'
    Start-Process -FilePath "powershell.exe" -Verb RunAs -WindowStyle Hidden -ArgumentList $arguments
    exit
}
$rulePrefix = "bobodrop-" + (Get-FileHash -LiteralPath $resolvedExecutable -Algorithm SHA256).Hash.Substring(0,12)
# Remove only matching rules for this executable; never touch other firewall rules.
Get-NetFirewallRule -ErrorAction SilentlyContinue | Where-Object { $_.Name -like 'bobodrop-*' } | ForEach-Object {
    $appFilter = $_ | Get-NetFirewallApplicationFilter
    if ($appFilter.Program -eq $resolvedExecutable) { $_ | Remove-NetFirewallRule }
}
# The web port is fixed, while each media session gets a short-lived TCP relay
# port from Windows. Scope the rule to this executable rather than a port range.
New-NetFirewallRule -Name ($rulePrefix + "-Private") -DisplayName "bobodrop - Wi-Fi Private/Domain" -Direction Inbound -Action Allow -Program $resolvedExecutable -Protocol TCP -Profile Private,Domain | Out-Null
New-NetFirewallRule -Name ($rulePrefix + "-PublicLAN") -DisplayName "bobodrop - subnet lokal Public" -Direction Inbound -Action Allow -Program $resolvedExecutable -Protocol TCP -RemoteAddress LocalSubnet -Profile Public | Out-Null
Write-Host "TCP bobodrop diizinkan untuk web dan relay media lokal. Firewall tetap aktif."
"TCP bobodrop diizinkan untuk web dan relay media lokal. Firewall tetap aktif." | Set-Content -LiteralPath (Join-Path $PSScriptRoot "Firewall-status.txt") -Encoding utf8
