param(
    [Parameter(Mandatory=$true)]
    [string]$OutputPath,

    [Parameter(Mandatory=$false)]
    [int]$X = -1,

    [Parameter(Mandatory=$false)]
    [int]$Y = -1,

    [Parameter(Mandatory=$false)]
    [int]$Width = -1,

    [Parameter(Mandatory=$false)]
    [int]$Height = -1
)

$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$screen = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds

if ($X -lt 0) {
    $X = 0
    $Y = 0
    $Width = $screen.Width
    $Height = $screen.Height
}

$bmp = New-Object System.Drawing.Bitmap($Width, $Height)
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($X, $Y, 0, 0, [System.Drawing.Size]::new($Width, $Height))
$bmp.Save($OutputPath)
$g.Dispose()
$bmp.Dispose()

Write-Host "Screenshot saved: $OutputPath"
