package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Security SecurityConfig `yaml:"security"`
	MC       MCConfig       `yaml:"mc"`
	Data      DataConfig      `yaml:"data"`
	BlueMap   BlueMapConfig   `yaml:"bluemap"`
	AutoSleep AutoSleepConfig `yaml:"auto_sleep"`
}

// AutoSleepConfig holds settings for sleep proxy and idle auto-shutdown.
type AutoSleepConfig struct {
	Enabled            bool   `yaml:"enabled"`
	Port               int    `yaml:"port"`
	IdleTimeoutMinutes int    `yaml:"idle_timeout_minutes"`
	WakeMOTD           string `yaml:"wake_motd"`
	WakeMessage        string `yaml:"wake_message"`
}

// ServerConfig holds web server settings.
type ServerConfig struct {
	Port int       `yaml:"port"`
	Host string    `yaml:"host"`
	TLS  TLSConfig `yaml:"tls"`
}

// TLSConfig holds HTTPS / TLS settings.
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
	AutoCert bool   `yaml:"auto_cert"`
}

// SecurityConfig holds authentication credentials and JWT secret.
type SecurityConfig struct {
	UserPassword  string `yaml:"user_password"`
	AdminPassword string `yaml:"admin_password"`
	JWTSecret     string `yaml:"jwt_secret"`
}

// MCConfig holds Minecraft server connection settings, file paths, and process parameters.
type MCConfig struct {
	RconAddress    string `yaml:"rcon_address"`
	RconPassword   string `yaml:"rcon_password"`
	PropertiesPath string `yaml:"properties_path"`
	ServerDir      string `yaml:"server_dir"`
	JarName        string `yaml:"jar_name"`
	JavaPath       string `yaml:"java_path"`
	MinMemory      string `yaml:"min_memory"`
	MaxMemory      string `yaml:"max_memory"`
}

// DataConfig holds web manager runtime storage locations.
type DataConfig struct {
	BackupDir string `yaml:"backup_dir"`
}

// BlueMapConfig holds BlueMap web integration settings.
type BlueMapConfig struct {
	URL string `yaml:"url"`
}

// Load reads configuration from the given file path.
func Load(path string) (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
			TLS: TLSConfig{
				Enabled:  false,
				CertFile: "./data/cert.pem",
				KeyFile:  "./data/key.pem",
				AutoCert: true,
			},
		},
		Security: SecurityConfig{
			UserPassword:  "minecraft",
			AdminPassword: "admin_minecraft",
			JWTSecret:     "mc-server-manager-secret-token",
		},
		MC: MCConfig{
			RconAddress:    "127.0.0.1:25575",
			RconPassword:   "rcon_password",
			PropertiesPath: "./server/server.properties",
			ServerDir:      "./server",
			JarName:        "purpur.jar",
			JavaPath:       "java",
			MinMemory:      "512M",
			MaxMemory:      "4G",
		},
		Data: DataConfig{
			BackupDir: "./data/backups",
		},
		BlueMap: BlueMapConfig{
			URL: "http://127.0.0.1:8100",
		},
		AutoSleep: AutoSleepConfig{
			Enabled:            true,
			Port:               25565,
			IdleTimeoutMinutes: 10,
			WakeMOTD:           "§6[Sanctum Server] §e절전 모드 대기 중\n§a접속 시 자동으로 서버가 켜집니다",
			WakeMessage:        "§6[Sanctum Server]§r\n\n§e서버가 절전 모드에서 기동 중입니다! (약 8초 소요)\n§a잠시 후 다시 접속해주세요.",
		},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return cfg, nil
}
