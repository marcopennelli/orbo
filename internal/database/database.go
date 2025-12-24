package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Database handles SQLite database operations
type Database struct {
	db *sql.DB
}

// CameraRecord represents a camera stored in the database
type CameraRecord struct {
	ID         string
	Name       string
	Device     string
	Resolution string
	FPS        int
	Status     string
	CreatedAt  time.Time
}

// MotionEventRecord represents a motion event stored in the database
type MotionEventRecord struct {
	ID               string
	CameraID         string
	Timestamp        time.Time
	Confidence       float64
	BoundingBoxes    []BoundingBoxRecord
	FramePath        string
	NotificationSent bool
	ObjectClass      string
	ObjectConfidence float64
	ThreatLevel      string
	InferenceTimeMs  float64
	DetectionDevice  string
}

// BoundingBoxRecord represents a bounding box
type BoundingBoxRecord struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// ConfigRecord represents a configuration key-value pair
type ConfigRecord struct {
	Key       string
	Value     string
	UpdatedAt time.Time
}

// New creates a new database connection
func New(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrent access
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	return &Database{db: db}, nil
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}

// Migrate runs database migrations
func (d *Database) Migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS cameras (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			device TEXT NOT NULL,
			resolution TEXT,
			fps INTEGER DEFAULT 30,
			status TEXT DEFAULT 'inactive',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS motion_events (
			id TEXT PRIMARY KEY,
			camera_id TEXT NOT NULL,
			timestamp DATETIME NOT NULL,
			confidence REAL,
			bounding_boxes TEXT,
			frame_path TEXT,
			notification_sent INTEGER DEFAULT 0,
			object_class TEXT,
			object_confidence REAL,
			threat_level TEXT,
			inference_time_ms REAL,
			detection_device TEXT,
			FOREIGN KEY (camera_id) REFERENCES cameras(id)
		)`,
		`CREATE TABLE IF NOT EXISTS app_config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_events_camera_time ON motion_events(camera_id, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_events_time ON motion_events(timestamp DESC)`,
	}

	for _, migration := range migrations {
		if _, err := d.db.Exec(migration); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	fmt.Println("Database migrations completed successfully")
	return nil
}

// SaveCamera saves or updates a camera
func (d *Database) SaveCamera(cam *CameraRecord) error {
	query := `INSERT INTO cameras (id, name, device, resolution, fps, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			device = excluded.device,
			resolution = excluded.resolution,
			fps = excluded.fps,
			status = excluded.status`

	_, err := d.db.Exec(query, cam.ID, cam.Name, cam.Device, cam.Resolution, cam.FPS, cam.Status, cam.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to save camera: %w", err)
	}
	return nil
}

// GetCamera retrieves a camera by ID
func (d *Database) GetCamera(id string) (*CameraRecord, error) {
	query := `SELECT id, name, device, resolution, fps, status, created_at FROM cameras WHERE id = ?`

	var cam CameraRecord
	err := d.db.QueryRow(query, id).Scan(&cam.ID, &cam.Name, &cam.Device, &cam.Resolution, &cam.FPS, &cam.Status, &cam.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get camera: %w", err)
	}
	return &cam, nil
}

// ListCameras returns all cameras
func (d *Database) ListCameras() ([]*CameraRecord, error) {
	query := `SELECT id, name, device, resolution, fps, status, created_at FROM cameras ORDER BY created_at DESC`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list cameras: %w", err)
	}
	defer rows.Close()

	var cameras []*CameraRecord
	for rows.Next() {
		var cam CameraRecord
		if err := rows.Scan(&cam.ID, &cam.Name, &cam.Device, &cam.Resolution, &cam.FPS, &cam.Status, &cam.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan camera: %w", err)
		}
		cameras = append(cameras, &cam)
	}
	return cameras, nil
}

// DeleteCamera deletes a camera by ID
func (d *Database) DeleteCamera(id string) error {
	_, err := d.db.Exec("DELETE FROM cameras WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete camera: %w", err)
	}
	return nil
}

// UpdateCameraStatus updates only the status of a camera
func (d *Database) UpdateCameraStatus(id, status string) error {
	_, err := d.db.Exec("UPDATE cameras SET status = ? WHERE id = ?", status, id)
	if err != nil {
		return fmt.Errorf("failed to update camera status: %w", err)
	}
	return nil
}

// SaveMotionEvent saves a motion event
func (d *Database) SaveMotionEvent(event *MotionEventRecord) error {
	bboxJSON, err := json.Marshal(event.BoundingBoxes)
	if err != nil {
		return fmt.Errorf("failed to marshal bounding boxes: %w", err)
	}

	notificationSent := 0
	if event.NotificationSent {
		notificationSent = 1
	}

	query := `INSERT INTO motion_events
		(id, camera_id, timestamp, confidence, bounding_boxes, frame_path, notification_sent,
		 object_class, object_confidence, threat_level, inference_time_ms, detection_device)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			notification_sent = excluded.notification_sent`

	_, err = d.db.Exec(query, event.ID, event.CameraID, event.Timestamp, event.Confidence,
		string(bboxJSON), event.FramePath, notificationSent, event.ObjectClass,
		event.ObjectConfidence, event.ThreatLevel, event.InferenceTimeMs, event.DetectionDevice)
	if err != nil {
		return fmt.Errorf("failed to save motion event: %w", err)
	}
	return nil
}

// GetMotionEvent retrieves a motion event by ID
func (d *Database) GetMotionEvent(id string) (*MotionEventRecord, error) {
	query := `SELECT id, camera_id, timestamp, confidence, bounding_boxes, frame_path,
		notification_sent, object_class, object_confidence, threat_level, inference_time_ms, detection_device
		FROM motion_events WHERE id = ?`

	var event MotionEventRecord
	var bboxJSON string
	var notificationSent int

	err := d.db.QueryRow(query, id).Scan(&event.ID, &event.CameraID, &event.Timestamp,
		&event.Confidence, &bboxJSON, &event.FramePath, &notificationSent,
		&event.ObjectClass, &event.ObjectConfidence, &event.ThreatLevel,
		&event.InferenceTimeMs, &event.DetectionDevice)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get motion event: %w", err)
	}

	event.NotificationSent = notificationSent == 1
	if bboxJSON != "" {
		if err := json.Unmarshal([]byte(bboxJSON), &event.BoundingBoxes); err != nil {
			return nil, fmt.Errorf("failed to unmarshal bounding boxes: %w", err)
		}
	}

	return &event, nil
}

// ListMotionEvents returns motion events with optional filtering
func (d *Database) ListMotionEvents(cameraID string, since *time.Time, limit int) ([]*MotionEventRecord, error) {
	query := `SELECT id, camera_id, timestamp, confidence, bounding_boxes, frame_path,
		notification_sent, object_class, object_confidence, threat_level, inference_time_ms, detection_device
		FROM motion_events WHERE 1=1`
	args := []interface{}{}

	if cameraID != "" {
		query += " AND camera_id = ?"
		args = append(args, cameraID)
	}

	if since != nil {
		query += " AND timestamp >= ?"
		args = append(args, *since)
	}

	query += " ORDER BY timestamp DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list motion events: %w", err)
	}
	defer rows.Close()

	var events []*MotionEventRecord
	for rows.Next() {
		var event MotionEventRecord
		var bboxJSON string
		var notificationSent int

		if err := rows.Scan(&event.ID, &event.CameraID, &event.Timestamp, &event.Confidence,
			&bboxJSON, &event.FramePath, &notificationSent, &event.ObjectClass,
			&event.ObjectConfidence, &event.ThreatLevel, &event.InferenceTimeMs, &event.DetectionDevice); err != nil {
			return nil, fmt.Errorf("failed to scan motion event: %w", err)
		}

		event.NotificationSent = notificationSent == 1
		if bboxJSON != "" {
			if err := json.Unmarshal([]byte(bboxJSON), &event.BoundingBoxes); err != nil {
				return nil, fmt.Errorf("failed to unmarshal bounding boxes: %w", err)
			}
		}
		events = append(events, &event)
	}
	return events, nil
}

// DeleteOldMotionEvents deletes events older than the specified time
func (d *Database) DeleteOldMotionEvents(before time.Time) (int64, error) {
	result, err := d.db.Exec("DELETE FROM motion_events WHERE timestamp < ?", before)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old motion events: %w", err)
	}
	return result.RowsAffected()
}

// SaveConfig saves a configuration value
func (d *Database) SaveConfig(key, value string) error {
	query := `INSERT INTO app_config (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP`

	_, err := d.db.Exec(query, key, value)
	if err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	return nil
}

// GetConfig retrieves a configuration value
func (d *Database) GetConfig(key string) (string, error) {
	var value string
	err := d.db.QueryRow("SELECT value FROM app_config WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get config: %w", err)
	}
	return value, nil
}

// ListConfigs returns all configuration values
func (d *Database) ListConfigs() (map[string]string, error) {
	rows, err := d.db.Query("SELECT key, value FROM app_config")
	if err != nil {
		return nil, fmt.Errorf("failed to list configs: %w", err)
	}
	defer rows.Close()

	configs := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("failed to scan config: %w", err)
		}
		configs[key] = value
	}
	return configs, nil
}

// DeleteConfig deletes a configuration value
func (d *Database) DeleteConfig(key string) error {
	_, err := d.db.Exec("DELETE FROM app_config WHERE key = ?", key)
	if err != nil {
		return fmt.Errorf("failed to delete config: %w", err)
	}
	return nil
}
