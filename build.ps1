#!/usr/bin/env pwsh
<#!
build.ps1 - Build the EpGo Docker image (multi-arch aware).

Usage examples:
  ./build.ps1                              # prompts for REF and tag values, loads native arch locally
  ./build.ps1 -Ref v1.3-RC -Tag dev        # passes REF and tag via parameters
  ./build.ps1 -Ref main -Tag qa -Push      # build and push multi-arch (amd64/arm64)
  ./build.ps1 -Ref main -Tag qa -Push -LatestTag   # also push the "latest" tag
  ./build.ps1 -Ref main -Tag qa -Push -DevelopTag  # also push the "develop" tag

Parameters:
  -Ref   Build argument for REF (required; prompted if omitted)
  -Tag   Image tag to apply to nillivanilli0815/epgo (required; prompted if omitted)
  -Push  Build and push a multi-arch image (amd64, arm64) to the registry
  -LatestTag  Also tag the image as "latest" (use with -Push)
  -DevelopTag Also tag the image as "develop" (use with -Push)
!>

param(
  [string]$Ref,
  [string]$Tag,
  [switch]$Push,
  [switch]$LatestTag,
  [switch]$DevelopTag
)

$ImageName = "nillivanilli0815/epgo"
$Platforms = "linux/amd64,linux/arm64"
$BuilderName = "epgo-builder"

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

$tagArgs = @('-t', "$ImageName:$Tag")
$tagList = @("$ImageName:$Tag")
if ($LatestTag) {
  $tagArgs += @('-t', "$ImageName:latest")
  $tagList += "$ImageName:latest"
}
if ($DevelopTag) {
  $tagArgs += @('-t', "$ImageName:develop")
  $tagList += "$ImageName:develop"
}
$tagString = $tagList -join ', '

if (-not (docker buildx inspect $BuilderName 2>$null)) {
  docker buildx create --name $BuilderName --use | Out-Null
} else {
  docker buildx use $BuilderName | Out-Null
}

if ($Push) {
  $buildArgs = @(
    '--platform', $Platforms,
    '--build-arg', "REF=$Ref"
  ) + $tagArgs + @('--push', '.')
  Write-Host "Building and pushing multi-arch image(s) $tagString for $Platforms with REF=$Ref..."
  docker buildx build @buildArgs
} else {
  $nativePlatform = docker info --format '{{.OSType}}/{{.Architecture}}'
  $buildArgs = @(
    '--platform', $nativePlatform,
    '--build-arg', "REF=$Ref"
  ) + $tagArgs + @('--load', '.')
  Write-Host "Building native image(s) $tagString for $nativePlatform with REF=$Ref (use -Push for multi-arch)..."
  docker buildx build @buildArgs
}

Write-Host "Image build completed."
