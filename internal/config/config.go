package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	JWT      JWTConfig
	Redis    RedisConfig
	MinIO    MinIOConfig
	TURN     TURNConfig
	Env      string
}
type MinIOConfig struct {
	Endpoint   string
	AccessKey  string
	SecretKey  string
	Bucket     string
	UseSSL     bool
	PublicHost string
}

type JWTConfig struct {
	AccessMinutes int
	RefreshDays   int
	AccessSecret  string
	RefreshSecret string
}

type ServerConfig struct {
	Port string
	Host string
}
type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func (d DatabaseConfig) DSN() string {
	return "host=" + d.Host +
		" port=" + d.Port +
		" user=" + d.User +
		" password=" + d.Password +
		" dbname=" + d.DBName +
		" sslmode=" + d.SSLMode
}

type RedisConfig struct {
	URL string
}
type TURNConfig struct {
	Host       string `env:"TURN_HOST"        envDefault:"turn.yourdomain.com"`
	Port       int    `env:"TURN_PORT"        envDefault:"3478"`
	TLSPort    int    `env:"TURN_TLS_PORT"    envDefault:"5349"`
	AuthSecret string `env:"TURN_AUTH_SECRET" envDefault:""`
	TTL        int    `env:"TURN_TTL"         envDefault:"86400"`
}

func Load() *Config {
	_ = godotenv.Load()
	return &Config{
		Env: getEnv("ENV", "development"),
		Server: ServerConfig{
			Port: getEnv("SERVER_PORT", ""),
			Host: getEnv("SERVER_HOST", ""),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", ""),
			Port:     getEnv("DB_PORT", ""),
			User:     getEnv("DB_USER", ""),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", ""),
			SSLMode:  getEnv("DB_SSLMODE", ""),
		},
		JWT: JWTConfig{
			AccessSecret:  getEnv("JWT_ACCESS_SECRET", ""),
			RefreshSecret: getEnv("JWT_REFRESH_SECRET", ""),
			AccessMinutes: getEnvInt("JWT_ACCESS_MINUTES", 15),
			RefreshDays:   getEnvInt("JWT_REFRESH_DAYS", 30),
		},
		Redis: RedisConfig{
			URL: getEnv("REDIS_URL", ""),
		},
		MinIO: MinIOConfig{
			Endpoint:   getEnv("MINIO_ENDPOINT", ""),
			AccessKey:  getEnv("MINIO_ACCESS_KEY", ""),
			SecretKey:  getEnv("MINIO_SECRET_KEY", ""),
			Bucket:     getEnv("MINIO_BUCKET", ""),
			UseSSL:     getEnvBool("MINIO_USE_SSL", false),
			PublicHost: getEnv("MINIO_PUBLIC_HOST", "http://"+getEnv("MINIO_ENDPOINT", "localhost:9000")),
		},
		TURN: TURNConfig{
			Host:       getEnv("TURN_HOST", "turn.yourdomain.com"),
			Port:       getEnvInt("TURN_PORT", 3478),
			TLSPort:    getEnvInt("TURN_TLS_PORT", 5349),
			AuthSecret: getEnv("TURN_AUTH_SECRET", ""),
			TTL:        getEnvInt("TURN_TTL", 86400),
		},
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}
func getEnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
