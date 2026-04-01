$ErrorActionPreference = "SilentlyContinue"

Add-Type -AssemblyName System.Windows.Forms

# 方法：使用 Win+B 快捷键聚焦到系统托盘，然后按空格或回车展开
# Win+B 会将焦点移动到系统托盘的第一个图标

# 先按 Esc 确保没有其他菜单打开
[System.Windows.Forms.SendKeys]::SendWait('{ESC}')
Start-Sleep -Milliseconds 200

# 按 Win+B 聚焦到系统托盘
[System.Windows.Forms.SendKeys]::SendWait('^{ESC}')  # Ctrl+Esc 打开开始菜单
Start-Sleep -Milliseconds 200
[System.Windows.Forms.SendKeys]::SendWait('{ESC}')   # Esc 关闭开始菜单
Start-Sleep -Milliseconds 200
[System.Windows.Forms.SendKeys]::SendWait('%^{b}')   # Alt+Ctrl+B 聚焦托盘
Start-Sleep -Milliseconds 300

# 按空格键展开隐藏的图标
[System.Windows.Forms.SendKeys]::SendWait(' ')
Start-Sleep -Milliseconds 500

Write-Host "Tray icons expanded"
