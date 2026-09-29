# com0com peer echo process: writes back every byte received on the given port
# (simulates TX-RX loopback). Started hidden by test-windows.ps1 and stopped
# via Stop-Process after the tests. Uses .NET System.IO.Ports (no pyserial).
param(
    [Parameter(Mandatory = $true)][string]$Port,
    [int]$Baud = 115200
)
$ErrorActionPreference = 'Stop'
$sp = New-Object System.IO.Ports.SerialPort
$sp.PortName = $Port
$sp.BaudRate = $Baud
$sp.Parity = 'None'
$sp.DataBits = 8
$sp.StopBits = 'One'
$sp.ReadTimeout = 200
$sp.Open()
try {
    while ($true) {
        try {
            $n = $sp.BytesToRead
            if ($n -gt 0) {
                $buf = New-Object byte[] $n
                $null = $sp.Read($buf, 0, $n)
                $sp.Write($buf, 0, $n)
            }
            else {
                Start-Sleep -Milliseconds 20
            }
        }
        catch [TimeoutException] {
            # read timeout is normal polling
        }
    }
}
finally {
    $sp.Close()
}
