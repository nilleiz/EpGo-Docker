#!/usr/bin/env pwsh
<#!
build.ps1 - Build the EpGo Docker image.

Usage examples:
  ./build.ps1                     # prompts for REF and tag values
  ./build.ps1 -Ref v1.3-RC -Tag dev
  ./build.ps1 -Ref main -Tag qa

Parameters:
  -Ref  Build argument for REF (required; prompted if omitted)
  -Tag  Image tag to apply to nillivanilli0815/epgo (required; prompted if omitted)
!>

param(
  [string]$Ref,
  [string]$Tag
)

$ImageName = "nillivanilli0815/epgo"

if (-not $Ref) {
  $Ref = Read-Host "Enter REF build argument (e.g., v1.3-RC)"
}

if (-not $Tag) {
  $Tag = Read-Host "Enter image tag to use for $ImageName (e.g., develop)"
}

if (-not $Ref -or -not $Tag) {
  Write-Error "Both REF and tag are required."
  exit 1
}

Write-Host "Building image $ImageName:$Tag with REF=$Ref..."
docker build --no-cache --build-arg "REF=$Ref" -t "$ImageName:$Tag" .
