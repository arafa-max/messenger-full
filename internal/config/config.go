package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Server         ServerConfig
	Database       DatabaseConfig
	JWT            JWTConfig
	Redis          RedisConfig
	MinIO          MinIOConfig
	TURN           TURNConfig
	Env            string
	Tenor          TenorConfig
	AI             AIConfig
	SD             StableDiffusionConfig
	Whisper        WhisperConfig
	Piper          PiperConfig
	LibreTranslate LibreTranslateConfig
	Stripe         StripeConfig
	Sentry         SentryConfig
	Features       FeatureFlags
	OAuth          OAuthConfig
	Email          EmailConfig
}
type EmailConfig struct {
	ResendAPIKey string
	From         string
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
	Port           string
	Host           string
	PublicURL      string
	WebAuthnRPID   string
	WebAuthnOrigin string
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
	Host       string
	Port       int
	TLSPort    int
	AuthSecret string
	TTL        int
}

type TenorConfig struct {
	APIKey string
}

type AIConfig struct {
	OllamaURL       string
	SmartReplyModel string
	SummaryModel    string
	WhisperURL      string
	MaxPromptChars  int
	RequestTimeout  int
}

type StableDiffusionConfig struct {
	URL     string
	Timeout int
}

type WhisperConfig struct {
	URL     string
	Timeout int
}

type PiperConfig struct {
	URL     string
	Timeout int
}

type LibreTranslateConfig struct {
	URL     string
	APIKey  string
	Timeout int
}

type StripeConfig struct {
	SecretKey      string
	WebhookSecret  string
	PremiumPriceID string
}

type SentryConfig struct {
	DSN string
}

type FeatureFlags struct {
	AI         bool
	Payments   bool
	Stories    bool
	Bots       bool
	E2EE       bool
	VoiceRooms bool
	ImageGen   bool
}

type OAuthConfig struct {
	Google OAuthProviderConfig
	GitHub OAuthProviderConfig
}

type OAuthProviderConfig struct {
	ClientID     string
	ClientSecret string
}

func Load() *Config {
	_ = godotenv.Load()
	return &Config{
		Env: getEnv("ENV", "development"),
		Server: ServerConfig{
			Port:           getEnv("SERVER_PORT", "8080"),
			Host:           getEnv("SERVER_HOST", "0.0.0.0"),
			PublicURL:      getEnv("PUBLIC_URL", "http://localhost:8080"),
			WebAuthnRPID:   getEnv("WEBAUTHN_RP_ID", "localhost"),
			WebAuthnOrigin: getEnv("WEBAUTHN_ORIGIN", "http://localhost:3000"),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", ""),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", ""),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		JWT: JWTConfig{
			AccessSecret:  getEnv("JWT_ACCESS_SECRET", ""),
			RefreshSecret: getEnv("JWT_REFRESH_SECRET", ""),
			AccessMinutes: getEnvInt("JWT_ACCESS_MINUTES", 15),
			RefreshDays:   getEnvInt("JWT_REFRESH_DAYS", 30),
		},
		Redis: RedisConfig{
			URL: getEnv("REDIS_URL", "redis://localhost:6379"),
		},
		MinIO: MinIOConfig{
			Endpoint:   getEnv("MINIO_ENDPOINT", "localhost:9000"),
			AccessKey:  getEnv("MINIO_ACCESS_KEY", ""),
			SecretKey:  getEnv("MINIO_SECRET_KEY", ""),
			Bucket:     getEnv("MINIO_BUCKET", "messenger-media"),
			UseSSL:     getEnvBool("MINIO_USE_SSL", false),
			PublicHost: getEnv("MINIO_PUBLIC_HOST", "http://"+getEnv("MINIO_ENDPOINT", "localhost:9000")),
		},
		TURN: TURNConfig{
			Host:       getEnv("TURN_HOST", ""),
			Port:       getEnvInt("TURN_PORT", 3478),
			TLSPort:    getEnvInt("TURN_TLS_PORT", 5349),
			AuthSecret: getEnv("TURN_AUTH_SECRET", ""),
			TTL:        getEnvInt("TURN_TTL", 86400),
		},
		Tenor: TenorConfig{
			APIKey: getEnv("TENOR_API_KEY", ""),
		},
		AI: AIConfig{
			OllamaURL:       getEnv("OLLAMA_URL", ""),
			SmartReplyModel: getEnv("AI_SMART_MODEL", "llama3.2:1b"),
			SummaryModel:    getEnv("AI_SUMMARY_MODEL", ""),
			WhisperURL:      getEnv("WHISPER_URL", ""),
			MaxPromptChars:  getEnvInt("AI_MAX_PROMPT_CHARS", 2000),
			RequestTimeout:  getEnvInt("AI_TIMEOUT_SEC", 30),
		},
		SD: StableDiffusionConfig{
			URL:     getEnv("SD_URL", ""),
			Timeout: getEnvInt("SD_TIMEOUT", 120),
		},
		Whisper: WhisperConfig{
			URL:     getEnv("WHISPER_URL", ""),
			Timeout: getEnvInt("WHISPER_TIMEOUT", 60),
		},
		Piper: PiperConfig{
			URL:     getEnv("PIPER_URL", ""),
			Timeout: getEnvInt("PIPER_TIMEOUT", 30),
		},
		LibreTranslate: LibreTranslateConfig{
			URL:     getEnv("LIBRETRANSLATE_URL", ""),
			APIKey:  getEnv("LIBRETRANSLATE_API_KEY", ""),
			Timeout: getEnvInt("LIBRETRANSLATE_TIMEOUT", 30),
		},
		Stripe: StripeConfig{
			SecretKey:      getEnv("STRIPE_SECRET_KEY", ""),
			WebhookSecret:  getEnv("STRIPE_WEBHOOK_SECRET", ""),
			PremiumPriceID: getEnv("STRIPE_PREMIUM_PRICE_ID", ""),
		},
		Sentry: SentryConfig{
			DSN: getEnv("SENTRY_DSN", ""),
		},
		Features: FeatureFlags{
			AI:         getEnvBool("FEATURE_AI", true),
			Payments:   getEnvBool("FEATURE_PAYMENTS", true),
			Stories:    getEnvBool("FEATURE_STORIES", true),
			Bots:       getEnvBool("FEATURE_BOTS", true),
			E2EE:       getEnvBool("FEATURE_E2EE", true),
			VoiceRooms: getEnvBool("FEATURE_VOICE_ROOMS", true),
			ImageGen:   getEnvBool("FEATURE_IMAGE_GEN", true),
		},
		OAuth: OAuthConfig{
			Google: OAuthProviderConfig{
				ClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
				ClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
			},
			GitHub: OAuthProviderConfig{
				ClientID:     getEnv("GITHUB_CLIENT_ID", ""),
				ClientSecret: getEnv("GITHUB_CLIENT_SECRET", ""),
			},
		},
		Email: EmailConfig{
			ResendAPIKey: getEnv("RESEND_API_KEY", ""),
			From:         getEnv("MAGIC_LINK_FROM", "noreply@yourdomain.com"),
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
