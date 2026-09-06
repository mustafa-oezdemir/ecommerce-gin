[CmdletBinding()]
param(
    [string]$AcmeEmail = "CHANGE_ME@example.com",
    [string]$SmtpHost = "CHANGE_ME.smtp.example.com",
    [string]$SmtpFrom = "CHANGE_ME@example.com",
    [string]$SmtpUsername = "CHANGE_ME",
    [string]$SmtpPassword = "CHANGE_ME",
    [switch]$Force
)

$ErrorActionPreference = "Stop"

function New-HexSecret([int]$ByteCount = 32) {
    $buffer = [byte[]]::new($ByteCount)
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($buffer)
    return [Convert]::ToHexString($buffer).ToLowerInvariant()
}

function New-Base64Secret([int]$ByteCount = 32) {
    $buffer = [byte[]]::new($ByteCount)
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($buffer)
    return [Convert]::ToBase64String($buffer)
}

$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$shippingRoot = (Resolve-Path (Join-Path $repositoryRoot "..\shipping-service")).Path
$ecommerceEnvPath = Join-Path $repositoryRoot ".env.production"
$shippingEnvPath = Join-Path $shippingRoot ".env.production"

foreach ($path in @($ecommerceEnvPath, $shippingEnvPath)) {
    if ((Test-Path -LiteralPath $path) -and -not $Force) {
        throw "$path already exists. Use -Force only when rotating every generated secret."
    }
}

$ecommerceDatabasePassword = New-HexSecret
$ecommerceRootPassword = New-HexSecret
$shippingDatabasePassword = New-HexSecret
$shippingRootPassword = New-HexSecret
$sessionSecret = New-Base64Secret 48
$csrfSecret = New-Base64Secret
$encryptionKey = New-Base64Secret
$paymentWebhookSecret = New-Base64Secret
$ecommerceToShippingToken = New-Base64Secret 48
$shippingToEcommerceToken = New-Base64Secret 48
$internalQRSecret = New-Base64Secret 48

$ecommerceEnvironment = @"
ACME_EMAIL=$AcmeEmail

ECOMMERCE_MYSQL_PASSWORD=$ecommerceDatabasePassword
ECOMMERCE_MYSQL_ROOT_PASSWORD=$ecommerceRootPassword
SHIPPING_MYSQL_PASSWORD=$shippingDatabasePassword
SHIPPING_MYSQL_ROOT_PASSWORD=$shippingRootPassword

SESSION_SECRET=$sessionSecret
CSRF_SECRET=$csrfSecret
SECURITY_ENCRYPTION_KEY=$encryptionKey
PAYMENT_WEBHOOK_SECRET=$paymentWebhookSecret
ECOMMERCE_TO_SHIPPING_TOKEN=$ecommerceToShippingToken
ECOMMERCE_TO_SHIPPING_PREVIOUS_TOKEN=
SHIPPING_TO_ECOMMERCE_TOKEN=$shippingToEcommerceToken
SHIPPING_TO_ECOMMERCE_PREVIOUS_TOKEN=
INTERNAL_QR_SECRET=$internalQRSecret

SMTP_HOST=$SmtpHost
SMTP_PORT=587
SMTP_FROM=$SmtpFrom
SMTP_USERNAME=$SmtpUsername
SMTP_PASSWORD=$SmtpPassword

WAREHOUSE_NAME=PehliOne Logistics Center
WAREHOUSE_COMPANY=PehliOne GmbH
WAREHOUSE_STREET=Musterstrasse
WAREHOUSE_HOUSE_NUMBER=10
WAREHOUSE_POSTAL_CODE=35039
WAREHOUSE_CITY=Marburg
WAREHOUSE_COUNTRY=DE
"@

$shippingEnvironment = @"
APP_ENV=production
APP_PORT=8090
APP_URL=https://pehlione-shipping.com
GIN_MODE=release
TRUSTED_PROXIES=10.231.17.0/24

DATABASE_DSN=shipping:$shippingDatabasePassword@tcp(shipping-db:3306)/shipping?charset=utf8mb4&parseTime=True&loc=UTC
DATABASE_CONNECT_TIMEOUT=30s
ECOMMERCE_TO_SHIPPING_TOKEN=$ecommerceToShippingToken
ECOMMERCE_TO_SHIPPING_PREVIOUS_TOKEN=
ECOMMERCE_API_URL=http://ecommerce-app:8080
ECOMMERCE_PUBLIC_URL=https://pehlione-ecommerce.com
ECOMMERCE_CALLBACK_URL=
SHIPPING_TO_ECOMMERCE_TOKEN=$shippingToEcommerceToken
INTERNAL_QR_SECRET=$internalQRSecret

WAREHOUSE_NAME=PehliOne Logistics Center
WAREHOUSE_COMPANY=PehliOne GmbH
WAREHOUSE_STREET=Musterstrasse
WAREHOUSE_HOUSE_NUMBER=10
WAREHOUSE_POSTAL_CODE=35039
WAREHOUSE_CITY=Marburg
WAREHOUSE_COUNTRY=DE
"@

Set-Content -LiteralPath $ecommerceEnvPath -Value $ecommerceEnvironment -Encoding utf8NoBOM
Set-Content -LiteralPath $shippingEnvPath -Value $shippingEnvironment -Encoding utf8NoBOM

Write-Host "Created ignored production environment files. Generated secrets were not printed."
Write-Host "Before deployment, replace every CHANGE_ME value with real ACME/SMTP settings."
