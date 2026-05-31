$bytes = New-Object Byte[] 24
$rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()

try {
    $rng.GetBytes($bytes)
}
finally {
    $rng.Dispose()
}

$key = [Convert]::ToBase64String($bytes).Replace("+", "-").Replace("/", "_")

if ($key.Length -ne 32) {
    throw "Generated COOKIE_ENCRYPTION_KEY must be exactly 32 characters."
}

Write-Output "COOKIE_ENCRYPTION_KEY=$key"
