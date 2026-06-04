param(
    [ValidateSet("jwt", "cookie", "collab", "all")]
    [string]$Kind = "all"
)

function New-HexSecret {
    $bytes = New-Object Byte[] 32
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()

    try {
        $rng.GetBytes($bytes)
    }
    finally {
        $rng.Dispose()
    }

    [System.BitConverter]::ToString($bytes).Replace("-", "").ToLower()
}

function New-CookieKey {
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

    $key
}

function Write-AppSecret {
    param([string]$SecretKind)

    switch ($SecretKind) {
        "jwt" {
            Write-Output "JWT_SECRET=$(New-HexSecret)"
        }
        "cookie" {
            Write-Output "COOKIE_ENCRYPTION_KEY=$(New-CookieKey)"
        }
        "collab" {
            Write-Output "COLLAB_TOKEN_SECRET=$(New-HexSecret)"
        }
        "all" {
            Write-AppSecret "jwt"
            Write-AppSecret "cookie"
            Write-AppSecret "collab"
        }
    }
}

Write-AppSecret $Kind
