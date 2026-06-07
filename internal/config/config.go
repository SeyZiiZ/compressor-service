package config

import (
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	RabbitMQ RabbitMQConfig
	Storage  StorageConfig
	Worker   WorkerConfig
	Webhook  WebhookConfig
}

type ServerConfig struct {
	Port        int    `mapstructure:"port"`
	Host        string `mapstructure:"host"`
	MaxUploadMB int    `mapstructure:"max_upload_mb"`
	// APIKey, when set, enables X-API-Key auth on /api/v1/jobs* (defense in depth).
	APIKey string `mapstructure:"api_key"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

func (d DatabaseConfig) DSN() string {
	return "host=" + d.Host +
		" port=" + strings.TrimSpace(viper.GetString("database.port")) +
		" user=" + d.User +
		" password=" + d.Password +
		" dbname=" + d.DBName +
		" sslmode=" + d.SSLMode
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type RabbitMQConfig struct {
	URL string `mapstructure:"url"`
}

type StorageConfig struct {
	// Driver selects the output backend: "local" (default) or "s3".
	Driver   string   `mapstructure:"driver"`
	BasePath string   `mapstructure:"base_path"`
	S3       S3Config `mapstructure:"s3"`
}

// S3Config holds credentials for an S3-compatible object store (e.g. a Railway Bucket).
type S3Config struct {
	Endpoint        string `mapstructure:"endpoint"`
	Region          string `mapstructure:"region"`
	Bucket          string `mapstructure:"bucket"`
	AccessKeyID     string `mapstructure:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key"`
	// ForcePathStyle = true for MinIO; false for Railway Buckets (virtual-hosted).
	ForcePathStyle bool `mapstructure:"force_path_style"`
}

// WebhookConfig controls the completion callback fired to the calling backend.
type WebhookConfig struct {
	Secret         string `mapstructure:"secret"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

type WorkerConfig struct {
	VideoWorkers int `mapstructure:"video_workers"`
	ImageWorkers int `mapstructure:"image_workers"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/etc/compressor/")

	// Defaults
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.max_upload_mb", 500)

	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.user", "compressor")
	viper.SetDefault("database.password", "compressor")
	viper.SetDefault("database.dbname", "compressor")
	viper.SetDefault("database.sslmode", "disable")

	viper.SetDefault("redis.addr", "localhost:6379")
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)

	viper.SetDefault("rabbitmq.url", "amqp://guest:guest@localhost:5672/")

	viper.SetDefault("storage.driver", "local")
	viper.SetDefault("storage.base_path", "./data")
	viper.SetDefault("storage.s3.endpoint", "")
	viper.SetDefault("storage.s3.region", "auto")
	viper.SetDefault("storage.s3.bucket", "")
	viper.SetDefault("storage.s3.access_key_id", "")
	viper.SetDefault("storage.s3.secret_access_key", "")
	viper.SetDefault("storage.s3.force_path_style", false)

	viper.SetDefault("worker.video_workers", 2)
	viper.SetDefault("worker.image_workers", 4)

	viper.SetDefault("server.api_key", "")
	viper.SetDefault("webhook.secret", "")
	viper.SetDefault("webhook.timeout_seconds", 15)

	// Env vars: COMPRESSOR_SERVER_PORT, COMPRESSOR_DATABASE_HOST, etc.
	viper.SetEnvPrefix("COMPRESSOR")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Try reading config file (optional)
	_ = viper.ReadInConfig()

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
