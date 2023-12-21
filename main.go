package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Nerzal/gocloak/v13"
	"github.com/gin-gonic/gin"

	"github.com/remiges-tech/alya/config"
	"github.com/remiges-tech/alya/logger"
	"github.com/remiges-tech/alya/router"
	"github.com/remiges-tech/alya/service"
	"github.com/remiges-tech/alya/wscutils"
	"github.com/remiges-tech/idshield/webservices/groupservice"
	"github.com/remiges-tech/logharbour/logharbour"
)

type AppConfig struct {
	AppServerPort        string `json:"app_server_port"`
	KeycloakURL          string `json:"keycloak_url"`
	KeycloakClientID     string `json:"keycloak_client_id"`
	KeycloakClientSecret string `json:"keycloak_client_secret"`
	ProviderURL          string `json:"provider_url"`
	Realm                string `json:"realm"`
}

func main() {
	configSystem := flag.String("configSource", "file", "The configuration system to use (file or rigel)")
	configFilePath := flag.String("configFile", "./config.json", "The path to the configuration file")
	rigelConfigName := flag.String("configName", "C1", "The name of the configuration")
	rigelSchemaName := flag.String("schemaName", "S1", "The name of the schema")
	etcdEndpoints := flag.String("etcdEndpoints", "localhost:2379", "Comma-separated list of etcd endpoints")

	flag.Parse()

	var appConfig AppConfig
	switch *configSystem {
	case "file":
		err := config.LoadConfigFromFile(*configFilePath, &appConfig)
		if err != nil {
			log.Fatalf("Error loading config: %v", err)
		}
	case "rigel":
		err := config.LoadConfigFromRigel(*etcdEndpoints, *rigelConfigName, *rigelSchemaName, &appConfig)
		if err != nil {
			log.Fatalf("Error loading config: %v", err)
		}
	default:
		log.Fatalf("Unknown configuration system: %s", *configSystem)
	}

	fmt.Printf("Loaded configuration: %+v\n", appConfig)

	// Open the error types file
	file, err := os.Open("./errortypes.yaml")
	if err != nil {
		log.Fatalf("Failed to open error types file: %v", err)
	}
	defer file.Close()

	// Load the error types
	wscutils.LoadErrorTypes(file)

	// logger
	// Open a file for logging.
	logFile, err := os.OpenFile("log.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()

	// Create a fallback writer that uses the file as the primary writer and stdout as the fallback.
	fallbackWriter := logharbour.NewFallbackWriter(logFile, os.Stdout)
	lctx := logharbour.NewLoggerContext(logharbour.Debug0)
	lh := logharbour.NewLogger(lctx, "Idshield", fallbackWriter)
	lh.WithPriority(logharbour.Info)
	fl := logger.NewFileLogger("/tmp/idshield.log")

	// auth middleware

	cache := router.NewRedisTokenCache("localhost:6379", "", 0, 0)
	authMiddleware, err := router.LoadAuthMiddleware(appConfig.KeycloakClientID, appConfig.ProviderURL, cache, fl)
	if err != nil {
		log.Fatalf("Failed to create new auth middleware: %v", err)
	}

	// router

	r, err := router.SetupRouter(true, fl, authMiddleware)
	if err != nil {
		log.Fatalf("Failed to setup router: %v", err)
	}

	// Logging middleware
	r.Use(func(c *gin.Context) {
		log.Printf("[request] %s - %s %s\n", c.Request.RemoteAddr, c.Request.Method, c.Request.URL.Path)
		start := time.Now()
		c.Next()
		duration := time.Since(start)
		log.Printf("[request] %s - %s %s %s\n", c.Request.RemoteAddr, c.Request.Method, c.Request.URL.Path, duration)
	})

	// create keycloak client
	client := gocloak.NewClient(appConfig.KeycloakURL)

	// Create a new service for /groups
	userService := service.NewService(r).WithLogHarbour(lh).WithDependency("goclock", client).WithDependency("realm", appConfig.Realm)

	// Register a route for handling group creation requests
	userService.RegisterRoute(http.MethodPost, "/capability-create", groupservice.HandleCapabilityCreateRequest)

	// Start the service
	if err := r.Run(":" + appConfig.AppServerPort); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
