package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	authService "orbo/gen/auth"
	cameraService "orbo/gen/camera"
	configService "orbo/gen/config"
	healthService "orbo/gen/health"
	motionService "orbo/gen/motion"
	systemService "orbo/gen/system"
	"orbo/internal/auth"
	"orbo/internal/camera"
	"orbo/internal/database"
	"orbo/internal/detection"
	"orbo/internal/motion"
	"orbo/internal/services"
	"orbo/internal/telegram"
	"orbo/internal/ws"
)

func main() {
	// Define command line flags, add any other flag required to configure the
	// service.
	var (
		hostF     = flag.String("host", "localhost", "Server host (valid values: localhost, production)")
		domainF   = flag.String("domain", "", "Host domain name (overrides host domain specified in service design)")
		httpPortF = flag.String("http-port", "", "HTTP port (overrides host HTTP port specified in service design)")
		secureF   = flag.Bool("secure", false, "Use secure scheme (https or grpcs)")
		dbgF      = flag.Bool("debug", false, "Log request and response bodies")
	)
	flag.Parse()

	// Setup logger. Replace logger with your own log package of choice.
	var (
		logger *log.Logger
	)
	{
		logger = log.New(os.Stderr, "[orbo] ", log.Ltime)
	}

	// Initialize database
	frameDir := os.Getenv("FRAME_DIR")
	if frameDir == "" {
		frameDir = "/app/frames"
	}
	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = filepath.Join(frameDir, "orbo.db")
	}

	db, err := database.New(dbPath)
	if err != nil {
		logger.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Run database migrations
	if err := db.Migrate(); err != nil {
		logger.Fatalf("Failed to run database migrations: %v", err)
	}
	logger.Printf("Database initialized at %s", dbPath)

	// Initialize camera manager
	cameraManager := camera.NewCameraManager(db)

	// Initialize motion detector
	motionDetector := motion.NewMotionDetector(frameDir, db)

	// Initialize Telegram bot
	telegramEnabled := os.Getenv("TELEGRAM_ENABLED") == "true"
	telegramCooldown := 30
	if cooldownStr := os.Getenv("TELEGRAM_COOLDOWN"); cooldownStr != "" {
		var cooldown int
		if _, err := fmt.Sscanf(cooldownStr, "%d", &cooldown); err == nil && cooldown > 0 {
			telegramCooldown = cooldown
		}
	}
	telegramBot := telegram.NewTelegramBot(telegram.Config{
		BotToken:        os.Getenv("TELEGRAM_BOT_TOKEN"),
		ChatID:          os.Getenv("TELEGRAM_CHAT_ID"),
		Enabled:         telegramEnabled && os.Getenv("TELEGRAM_BOT_TOKEN") != "" && os.Getenv("TELEGRAM_CHAT_ID") != "",
		CooldownSeconds: telegramCooldown,
	})
	if telegramEnabled {
		logger.Printf("Telegram notifications enabled (cooldown: %ds)", telegramCooldown)
	}

	// Initialize DINOv3 detector
	dinov3Endpoint := os.Getenv("DINOV3_ENDPOINT")
	if dinov3Endpoint == "" {
		dinov3Endpoint = "http://dinov3-service:8000"
	}
	dinov3Detector := detection.NewDINOv3Detector(dinov3Endpoint)

	// Initialize YOLO detector - prefer gRPC for low-latency streaming
	yoloGRPCEndpoint := os.Getenv("YOLO_GRPC_ENDPOINT")
	yoloHTTPEndpoint := os.Getenv("YOLO_ENDPOINT")
	if yoloHTTPEndpoint == "" {
		yoloHTTPEndpoint = "http://yolo-service:8081"
	}
	if yoloGRPCEndpoint == "" {
		yoloGRPCEndpoint = "yolo-service:50051"
	}
	drawBoxes := os.Getenv("YOLO_DRAW_BOXES") == "true"
	yoloEnabled := os.Getenv("YOLO_ENABLED") == "true"

	// Use gRPC detector for real-time streaming performance
	var grpcDetector *detection.GRPCDetector
	var yoloDetector *detection.YOLODetector
	if yoloEnabled {
		var err error
		grpcDetector, err = detection.NewGRPCDetector(detection.GRPCDetectorConfig{
			Endpoint:      yoloGRPCEndpoint,
			DrawBoxes:     drawBoxes,
			ConfThreshold: 0.5,
		})
		if err != nil {
			logger.Printf("Warning: Failed to connect to YOLO gRPC service: %v, falling back to HTTP", err)
			// Fall back to HTTP client
			yoloDetector = detection.NewYOLODetectorWithConfig(detection.YOLOConfig{
				Enabled:             true,
				ServiceEndpoint:     yoloHTTPEndpoint,
				ConfidenceThreshold: 0.5,
				SecurityMode:        true,
				DrawBoxes:           drawBoxes,
			})
		} else {
			logger.Printf("YOLO gRPC detector connected to %s", yoloGRPCEndpoint)
		}
	}

	// Create HTTP fallback detector for config service compatibility
	if yoloDetector == nil {
		yoloDetector = detection.NewYOLODetectorWithConfig(detection.YOLOConfig{
			Enabled:             yoloEnabled,
			ServiceEndpoint:     yoloHTTPEndpoint,
			ConfidenceThreshold: 0.5,
			SecurityMode:        true,
			DrawBoxes:           drawBoxes,
		})
	}

	// Apply initial draw boxes setting to motion detector
	motionDetector.SetDrawBoxes(drawBoxes)

	// Set gRPC detector on motion detector for low-latency streaming
	if grpcDetector != nil {
		motionDetector.SetGRPCDetector(grpcDetector)
		logger.Printf("Motion detector configured with gRPC YOLO")
	}

	// Set Telegram bot on motion detector for notifications
	motionDetector.SetTelegramBot(telegramBot)

	// Initialize WebSocket hub for real-time detections
	wsHub := ws.NewDetectionHub()
	motionDetector.SetWebSocketHub(wsHub)
	logger.Println("WebSocket hub initialized for real-time detections")

	// Initialize Face Recognition - prefer gRPC for low-latency streaming
	recognitionGRPCEndpoint := os.Getenv("RECOGNITION_GRPC_ENDPOINT")
	recognitionHTTPEndpoint := os.Getenv("RECOGNITION_SERVICE_ENDPOINT")
	if recognitionHTTPEndpoint == "" {
		recognitionHTTPEndpoint = "http://recognition-service:8082"
	}
	if recognitionGRPCEndpoint == "" {
		recognitionGRPCEndpoint = "recognition-service:50052"
	}
	recognitionEnabled := os.Getenv("RECOGNITION_ENABLED") == "true"

	var grpcFaceRecognizer *detection.GRPCFaceRecognizer
	if recognitionEnabled {
		var err error
		grpcFaceRecognizer, err = detection.NewGRPCFaceRecognizer(detection.GRPCFaceRecognizerConfig{
			Endpoint:            recognitionGRPCEndpoint,
			SimilarityThreshold: 0.5,
		})
		if err != nil {
			logger.Printf("Warning: Failed to connect to Face Recognition gRPC service: %v", err)
		} else {
			logger.Printf("Face Recognition gRPC connected to %s", recognitionGRPCEndpoint)
			// Set gRPC face recognizer on motion detector
			motionDetector.SetGRPCFaceRecognizer(grpcFaceRecognizer)
			logger.Printf("Motion detector configured with gRPC Face Recognition")
		}
	}

	// Log detector configuration
	logger.Printf("Detection configuration:")
	logger.Printf("  Primary detector: %s", os.Getenv("PRIMARY_DETECTOR"))
	logger.Printf("  DINOv3 endpoint: %s (enabled: %v)", dinov3Endpoint, os.Getenv("DINOV3_ENABLED") == "true")
	if grpcDetector != nil {
		logger.Printf("  YOLO gRPC endpoint: %s (enabled: %v, draw_boxes: %v)", yoloGRPCEndpoint, yoloEnabled, drawBoxes)
	} else {
		logger.Printf("  YOLO HTTP endpoint: %s (enabled: %v, draw_boxes: %v)", yoloHTTPEndpoint, yoloEnabled, drawBoxes)
	}
	if grpcFaceRecognizer != nil {
		logger.Printf("  Face Recognition gRPC endpoint: %s (enabled: %v)", recognitionGRPCEndpoint, recognitionEnabled)
	} else {
		logger.Printf("  Face Recognition HTTP endpoint: %s (enabled: %v)", recognitionHTTPEndpoint, recognitionEnabled)
	}

	// Initialize authenticator
	authenticator := auth.NewAuthenticator()
	if authenticator.IsEnabled() {
		logger.Printf("Authentication enabled (user: %s)", os.Getenv("AUTH_USERNAME"))
	} else {
		logger.Printf("Authentication disabled (set AUTH_ENABLED=true to enable)")
	}

	// Initialize the services.
	var (
		healthSvc healthService.Service
		authSvc   authService.Service
		cameraSvc cameraService.Service
		motionSvc motionService.Service
		configSvc configService.Service
		systemSvc systemService.Service
	)
	{
		healthSvc = services.NewHealthService()
		authSvc = services.NewAuthService(authenticator)
		cameraSvc = services.NewCameraService(cameraManager, motionDetector)
		motionSvc = services.NewMotionService(motionDetector, cameraManager)
		configSvc = services.NewConfigService(telegramBot, dinov3Detector, yoloDetector, motionDetector, db)
		// Create system service and wire up pipeline config getter
		systemImpl := services.NewSystemService(cameraManager, motionDetector)
		// ConfigImplementation implements PipelineConfigGetter interface
		if configImpl, ok := configSvc.(*services.ConfigImplementation); ok {
			systemImpl.SetPipelineConfigGetter(configImpl)
			// CRITICAL: Also wire up pipeline config to motion detector for mode gating
			// This is what actually controls whether YOLO runs or not based on mode
			motionDetector.SetPipelineConfig(services.CreatePipelineConfigProvider(configImpl))
			logger.Println("Pipeline config provider wired to motion detector")

			// Wire gRPC YOLO detector for multi-task support (detect, segment, pose, etc.)
			if grpcDetector != nil {
				configImpl.SetGRPCYoloDetector(grpcDetector)
				logger.Println("gRPC YOLO detector wired to config service for task updates")
			}
		}
		systemSvc = systemImpl
	}

	// Initialize Telegram command handler for bot commands
	commandHandler := telegram.NewCommandHandler(telegramBot, cameraManager, motionDetector, db)

	// Wrap the services in endpoints that can be invoked from other services
	// potentially running in different processes.
	var (
		healthEndpoints *healthService.Endpoints
		authEndpoints   *authService.Endpoints
		cameraEndpoints *cameraService.Endpoints
		motionEndpoints *motionService.Endpoints
		configEndpoints *configService.Endpoints
		systemEndpoints *systemService.Endpoints
	)
	{
		healthEndpoints = healthService.NewEndpoints(healthSvc)
		authEndpoints = authService.NewEndpoints(authSvc)
		cameraEndpoints = cameraService.NewEndpoints(cameraSvc)
		motionEndpoints = motionService.NewEndpoints(motionSvc)
		systemEndpoints = systemService.NewEndpoints(systemSvc)
		configEndpoints = configService.NewEndpoints(configSvc)
	}

	// Create channel used by both the signal handler and server goroutines
	// to notify the main goroutine when to stop the server.
	errc := make(chan error)

	// Setup interrupt handler. This optional step configures the process so
	// that SIGINT and SIGTERM signals cause the services to stop gracefully.
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errc <- fmt.Errorf("%s", <-c)
	}()

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	// Start Telegram command handler if bot is enabled
	if telegramBot.IsEnabled() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := commandHandler.StartPolling(ctx); err != nil {
				logger.Printf("Telegram command handler error: %v", err)
			}
		}()
		logger.Println("Telegram command handler started")
	}

	// Start the servers and send errors (if any) to the error channel.
	switch *hostF {
	case "localhost", "0.0.0.0":
		{
			addr := fmt.Sprintf("http://%s:8080", *hostF)
			u, err := url.Parse(addr)
			if err != nil {
				logger.Fatalf("invalid URL %#v: %s\n", addr, err)
			}
			if *secureF {
				u.Scheme = "https"
			}
			if *domainF != "" {
				u.Host = *domainF
			}
			if *httpPortF != "" {
				h, _, err := net.SplitHostPort(u.Host)
				if err != nil {
					logger.Fatalf("invalid URL %#v: %s\n", u.Host, err)
				}
				u.Host = net.JoinHostPort(h, *httpPortF)
			} else if u.Port() == "" {
				u.Host = net.JoinHostPort(u.Host, "80")
			}
			handleHTTPServer(ctx, u, healthEndpoints, authEndpoints, cameraEndpoints, motionEndpoints, configEndpoints, systemEndpoints, authenticator, cameraManager, motionDetector, wsHub, &wg, errc, logger, *dbgF)
		}

	case "production":
		{
			addr := "https://orbo.example.com"
			u, err := url.Parse(addr)
			if err != nil {
				logger.Fatalf("invalid URL %#v: %s\n", addr, err)
			}
			if *secureF {
				u.Scheme = "https"
			}
			if *domainF != "" {
				u.Host = *domainF
			}
			if *httpPortF != "" {
				h, _, err := net.SplitHostPort(u.Host)
				if err != nil {
					logger.Fatalf("invalid URL %#v: %s\n", u.Host, err)
				}
				u.Host = net.JoinHostPort(h, *httpPortF)
			} else if u.Port() == "" {
				u.Host = net.JoinHostPort(u.Host, "443")
			}
			handleHTTPServer(ctx, u, healthEndpoints, authEndpoints, cameraEndpoints, motionEndpoints, configEndpoints, systemEndpoints, authenticator, cameraManager, motionDetector, wsHub, &wg, errc, logger, *dbgF)
		}

	default:
		logger.Fatalf("invalid host argument: %q (valid hosts: localhost|0.0.0.0|production)\n", *hostF)
	}

	// Wait for signal.
	logger.Printf("exiting (%v)", <-errc)

	// Send cancellation signal to the goroutines.
	cancel()

	wg.Wait()
	logger.Println("exited")
}
