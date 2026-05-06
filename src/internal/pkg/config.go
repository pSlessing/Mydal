package pkg

import "os"

type Config struct {
	Addr        string
	DatabaseURL string
	BucketName  string
	LogLevel    string
}

func Load() Config {
	return Config{
		Addr:        os.Getenv("ADDR"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		BucketName:  os.Getenv("BUCKET_NAME"),
		LogLevel:    os.Getenv("LOG_LEVEL"),
	}
}
