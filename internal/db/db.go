package db

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/JoanixX/hospital-bed-prediction/internal/types"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	MongoClient *mongo.Client
	RedisClient *redis.Client
	MongoColl   *mongo.Collection
)

// InitDB inicializa las conexiones a MongoDB y Redis leyendo variables de entorno.
func InitDB() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Inicialización de MongoDB
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}
	log.Printf("[db] Conectando a MongoDB en: %s", mongoURI)
	mClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return fmt.Errorf("error al conectar con MongoDB: %w", err)
	}

	err = mClient.Ping(ctx, nil)
	if err != nil {
		return fmt.Errorf("ping fallido a MongoDB: %w", err)
	}

	MongoClient = mClient
	MongoColl = mClient.Database("hospital").Collection("predictions")
	log.Println("[db] Conexión a MongoDB establecida correctamente.")

	// 2. Inicialización de Redis
	redisURI := os.Getenv("REDIS_URI")
	if redisURI == "" {
		redisURI = "localhost:6379"
	}
	log.Printf("[db] Conectando a Redis en: %s", redisURI)
	opt, err := redis.ParseURL(redisURI)
	if err != nil {
		// Intentar fallback si es un formato host:port simple en lugar de redis://
		opt = &redis.Options{
			Addr: redisURI,
		}
	}

	rClient := redis.NewClient(opt)
	err = rClient.Ping(ctx).Err()
	if err != nil {
		return fmt.Errorf("ping fallido a Redis: %w", err)
	}

	RedisClient = rClient
	log.Println("[db] Conexión a Redis establecida correctamente.")

	return nil
}

// GenerateCacheKey genera una llave hash única para el paciente a partir de sus parámetros clínicos.
func GenerateCacheKey(p types.Patient) string {
	// Usamos los parámetros que influyen directamente en la predicción para evitar colisiones.
	return fmt.Sprintf("pred:%d:%s:%f:%f:%d:%d",
		p.Age,
		p.Race,
		p.PSALevel,
		p.Income,
		p.NumEncounters,
		p.NumDiagnoses,
	)
}

// GetCachedPrediction intenta recuperar un resultado de predicción desde Redis.
func GetCachedPrediction(ctx context.Context, key string) (*types.PatientResult, error) {
	if RedisClient == nil {
		return nil, fmt.Errorf("cliente Redis no inicializado")
	}

	val, err := RedisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil // Cache miss
	} else if err != nil {
		return nil, err
	}

	var res types.PatientResult
	err = json.Unmarshal([]byte(val), &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

// SetCachedPrediction guarda el resultado de predicción en la caché de Redis.
func SetCachedPrediction(ctx context.Context, key string, res types.PatientResult, expiration time.Duration) error {
	if RedisClient == nil {
		return fmt.Errorf("cliente Redis no inicializado")
	}

	data, err := json.Marshal(res)
	if err != nil {
		return err
	}

	return RedisClient.Set(ctx, key, data, expiration).Err()
}

// SavePrediction guarda el registro de predicción y el estado original del paciente en MongoDB.
func SavePrediction(ctx context.Context, p types.Patient, r types.PatientResult) error {
	if MongoColl == nil {
		return fmt.Errorf("colección de MongoDB no inicializada")
	}

	document := bson.M{
		"patientId":        r.PatientID,
		"patient":          p,
		"mortalityRisk":    r.MortalityRisk,
		"survivalEstimate": r.SurvivalEstimate,
		"treatmentCost":    r.TreatmentCost,
		"workerId":         r.WorkerID,
		"timestamp":        time.Now(),
	}

	_, err := MongoColl.InsertOne(ctx, document)
	return err
}
