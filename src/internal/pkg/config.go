package pkg

import "os"

type Config struct {
	Addr           string
	DatabaseURL    string
	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	MinioUseSSL    bool
	BucketName     string
	LogLevel       string
	Mode           string
}

func Load() Config {
	return Config{
		Addr:           os.Getenv("ADDR"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		MinioEndpoint:  os.Getenv("MINIO_ENDPOINT"),
		MinioAccessKey: os.Getenv("MINIO_ACCESS_KEY"),
		MinioSecretKey: os.Getenv("MINIO_SECRET_KEY"),
		MinioUseSSL:    os.Getenv("MINIO_USE_SSL") == "true",
		BucketName:     os.Getenv("BUCKET_NAME"),
		LogLevel:       os.Getenv("LOG_LEVEL"),
		Mode:           os.Getenv("MODE"),
	}
}
