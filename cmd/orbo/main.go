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

	cameraService "orbo/gen/camera"
	configService "orbo/gen/config"
	healthService "orbo/gen/health"
	motionService "orbo/gen/motion"
	systemService "orbo/gen/system"
	"orbo/internal/camera"
	"orbo/internal/database"
	"orbo/internal/detection"
	"orbo/internal/motion"
	"orbo/internal/services"
	"orbo/internal/telegram"
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

	// Initialize YOLO detector
	yoloEndpoint := os.Getenv("YOLO_ENDPOINT")
	if yoloEndpoint == "" {
		yoloEndpoint = "http://yolo-service:8081"
	}
	drawBoxes := os.Getenv("YOLO_DRAW_BOXES") == "true"
	yoloDetector := detection.NewYOLODetectorWithConfig(detection.YOLOConfig{
		Enabled:             os.Getenv("YOLO_ENABLED") == "true",
		ServiceEndpoint:     yoloEndpoint,
		ConfidenceThreshold: 0.5,
		SecurityMode:        true,
		DrawBoxes:           drawBoxes,
	})

	// Apply initial draw boxes setting to motion detector
	motionDetector.SetDrawBoxes(drawBoxes)

	// Set Telegram bot on motion detector for notifications
	motionDetector.SetTelegramBot(telegramBot)

	// Log detector configuration
	logger.Printf("Detection configuration:")
	logger.Printf("  Primary detector: %s", os.Getenv("PRIMARY_DETECTOR"))
	logger.Printf("  DINOv3 endpoint: %s (enabled: %v)", dinov3Endpoint, os.Getenv("DINOV3_ENABLED") == "true")
	logger.Printf("  YOLO endpoint: %s (enabled: %v, draw_boxes: %v)", yoloEndpoint, os.Getenv("YOLO_ENABLED") == "true", drawBoxes)

	// Initialize the services.
	var (
		healthSvc healthService.Service
		cameraSvc cameraService.Service
		motionSvc motionService.Service
		configSvc configService.Service
		systemSvc systemService.Service
	)
	{
		healthSvc = services.NewHealthService()
		cameraSvc = services.NewCameraService(cameraManager)
		motionSvc = services.NewMotionService(motionDetector, cameraManager)
		systemSvc = services.NewSystemService(cameraManager, motionDetector)
		configSvc = services.NewConfigService(telegramBot, dinov3Detector, yoloDetector, motionDetector, db)
	}

	// Wrap the services in endpoints that can be invoked from other services
	// potentially running in different processes.
	var (
		healthEndpoints *healthService.Endpoints
		cameraEndpoints *cameraService.Endpoints
		motionEndpoints *motionService.Endpoints
		configEndpoints *configService.Endpoints
		systemEndpoints *systemService.Endpoints
	)
	{
		healthEndpoints = healthService.NewEndpoints(healthSvc)
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
			handleHTTPServer(ctx, u, healthEndpoints, cameraEndpoints, motionEndpoints, configEndpoints, systemEndpoints, &wg, errc, logger, *dbgF)
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
			handleHTTPServer(ctx, u, healthEndpoints, cameraEndpoints, motionEndpoints, configEndpoints, systemEndpoints, &wg, errc, logger, *dbgF)
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
