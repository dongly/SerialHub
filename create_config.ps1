"$sb = [System.Text.StringBuilder]::new()
$sb.AppendLine('package config') | Out-Null
$sb.AppendLine('') | Out-Null
$sb.AppendLine('import (') | Out-Null
$sb.AppendLine('    "fmt"') | Out-Null
$sb.AppendLine('    "github.com/sirupsen/logrus"') | Out-Null
$sb.AppendLine('    "github.com/spf13/viper"') | Out-Null
$sb.AppendLine(')') | Out-Null
$sb.AppendLine('') | Out-Null
$sb.AppendLine('type SerialConfig struct {') | Out-Null
$sb.AppendLine('    Port     string + """json:"port""""') | Out-Null
$sb.AppendLine('    BaudRate int    + """json:"baudRate""""') | Out-Null
$sb.AppendLine('    DataBits int    + """json:"dataBits""""') | Out-Null
$sb.AppendLine('    Parity   string + """json:"parity""""') | Out-Null
$sb.AppendLine('    StopBits int    + """json:"stopBits""""') | Out-Null
$sb.AppendLine('}') | Out-Null
$sb.AppendLine('') | Out-Null
$sb.AppendLine('type TelnetConfig struct {') | Out-Null
$sb.AppendLine('    Port int + """json:"port""""') | Out-Null
$sb.AppendLine('}') | Out-Null
$sb.AppendLine('') | Out-Null
$sb.AppendLine('type MCPConfig struct {') | Out-Null
$sb.AppendLine('    HTTPPort int + """json:"httpPort""""') | Out-Null
$sb.AppendLine('}') | Out-Null
$sb.AppendLine('') | Out-Null
$sb.AppendLine('type Config struct {') | Out-Null
$sb.AppendLine('    Serial SerialConfig + """json:"serial""""') | Out-Null
$sb.AppendLine('    Telnet TelnetConfig + """json:"telnet""""') | Out-Null
$sb.AppendLine('    MCP    MCPConfig    + """json:"mcp""""') | Out-Null
$sb.AppendLine('    Debug  bool         + """json:"debug""""') | Out-Null
$sb.AppendLine('}') | Out-Null
$sb.AppendLine('') | Out-Null
$sb.AppendLine('func GetDefault() *Config {') | Out-Null
$sb.AppendLine('    return &Config{') | Out-Null
$sb.AppendLine('        Serial: SerialConfig{') | Out-Null
$sb.AppendLine('            Port:     "",') | Out-Null
$sb.AppendLine('            BaudRate: 115200,') | Out-Null
$sb.AppendLine('            DataBits: 8,') | Out-Null
$sb.AppendLine('            Parity:   "none",') | Out-Null
$sb.AppendLine('            StopBits: 1,') | Out-Null
$sb.AppendLine('        },') | Out-Null
$sb.AppendLine('        Telnet: TelnetConfig{') | Out-Null
$sb.AppendLine('            Port: 2323,') | Out-Null
$sb.AppendLine('        },') | Out-Null
$sb.AppendLine('        MCP: MCPConfig{') | Out-Null
$sb.AppendLine('            HTTPPort: 5000,') | Out-Null
$sb.AppendLine('        },') | Out-Null
$sb.AppendLine('        Debug: false,') | Out-Null
$sb.AppendLine('    }') | Out-Null
$sb.AppendLine('}') | Out-Null
$sb.AppendLine('') | Out-Null
$sb.AppendLine('func Load(configPath string) (*Config, error) {') | Out-Null
$sb.AppendLine('    cfg := GetDefault()') | Out-Null
$sb.AppendLine('    v := viper.New()') | Out-Null
$sb.AppendLine('    v.SetConfigFile(configPath)') | Out-Null
$sb.AppendLine('    ') | Out-Null
$sb.AppendLine('    if err := v.ReadInConfig(); err != nil {') | Out-Null
$sb.AppendLine('        if _, ok := err.(viper.ConfigFileNotFoundError); ok {') | Out-Null
$sb.AppendLine('            logrus.Warnf("[SerialHub] config file not found: %s", configPath)') | Out-Null
$sb.AppendLine('        } else {') | Out-Null
$sb.AppendLine('            return nil, fmt.Errorf("failed to read config file: %w", err)') | Out-Null
$sb.AppendLine('        }') | Out-Null
$sb.AppendLine('    } else {') | Out-Null
$sb.AppendLine('        logrus.Infof("[SerialHub] config loaded: %s", configPath)') | Out-Null
$sb.AppendLine('    }') | Out-Null
$sb.AppendLine('    ') | Out-Null
$sb.AppendLine('    if err := v.Unmarshal(cfg); err != nil {') | Out-Null
$sb.AppendLine('        return nil, fmt.Errorf("failed to parse config file: %w", err)') | Out-Null
$sb.AppendLine('    }') | Out-Null
$sb.AppendLine('    ') | Out-Null
$sb.AppendLine('    if cfg.Debug {') | Out-Null
$sb.AppendLine('        logrus.Debugf("[SerialHub] current config: %+v", cfg)') | Out-Null
$sb.AppendLine('    }') | Out-Null
$sb.AppendLine('    ') | Out-Null
$sb.AppendLine('    return cfg, nil') | Out-Null
$sb.AppendLine('}')
[System.IO.File]::WriteAllText('D:\Develop\SerialHub\pkg\config\config.go', $sb.ToString(), [System.Text.Encoding]::UTF8)
