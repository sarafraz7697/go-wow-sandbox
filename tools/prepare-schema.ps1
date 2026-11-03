param([Parameter(Mandatory=$true)][string]$DumpPath)
$ErrorActionPreference = 'Stop'
$source = [IO.File]::ReadAllText((Resolve-Path -LiteralPath $DumpPath))
if ($source -match '(?im)^\s*(INSERT|REPLACE|DROP|GRANT|CREATE USER|TRUNCATE)\b') {
    throw 'Expected a structure-only HeidiSQL dump. Review data-bearing dumps separately.'
}
$output = Join-Path $PSScriptRoot '../docker/init'
New-Item -ItemType Directory -Force -Path $output | Out-Null
$prefix = 'CREATE DATABASE IF NOT EXISTS `realmd`; CREATE DATABASE IF NOT EXISTS `mangos1`; CREATE DATABASE IF NOT EXISTS `character1`;'
$pattern = '(?m)^(-- Dumping structure for table (realmd|mangos1|character1)\.[^\r\n]+)'
$prepared = [regex]::Replace($source, $pattern, {param($match) 'USE `' + $match.Groups[2].Value + '`;' + "`n" + $match.Groups[1].Value})
if ($prepared -eq $source) { throw 'No recognized database/table markers found.' }
[IO.File]::WriteAllText((Join-Path $output '00-schema.sql'), $prefix + "`n" + $prepared, (New-Object Text.UTF8Encoding($false)))
Write-Output 'Prepared Docker-only schema import (no account/world data).'
