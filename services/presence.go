package services

import (
	"database/sql"
	"device-log/config"
	"log"
	"strings"
	"sync"
	"time"
)

type DeviceState struct {
	ID       string
	IsOnline bool
	LastPing time.Time
	Mu       sync.Mutex
}

type LogTask struct {
	DeviceID   string
	MacAddress string
	IsOnline   bool
}

var (
	deviceCache sync.Map
	logQueue    chan LogTask
	wg          sync.WaitGroup
	isClosing   bool
	closeMu     sync.Mutex
)

const (
	workerCount = 5
	queueSize   = 1000
)

// InitPresenceService initializes the presence cache by preloading all devices from DB and starting log workers
func InitPresenceService() {
	logQueue = make(chan LogTask, queueSize)
	isClosing = false

	// Start background log workers
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go logWorker(i)
	}

	// Pre-load all registered devices from DB to solve Cold-Start issue
	preloadDevicesFromDB()
}

func preloadDevicesFromDB() {
	query := `SELECT id, "macAddress", "lastSeen" FROM "Device"`
	rows, err := config.DB.Query(query)
	if err != nil {
		log.Printf("Failed to preload devices from database: %v\n", err)
		return
	}
	defer rows.Close()

	now := time.Now()
	count := 0

	for rows.Next() {
		var id, macAddress string
		var lastSeen sql.NullTime

		if err := rows.Scan(&id, &macAddress, &lastSeen); err != nil {
			log.Printf("Error scanning device row: %v\n", err)
			continue
		}

		macAddress = strings.TrimSpace(macAddress)
		if macAddress == "" {
			continue
		}

		lastPing := time.Time{}
		isOnline := false

		if lastSeen.Valid {
			lastPing = lastSeen.Time
			// If last ping was within the last 1 minute, consider online initially
			if now.Sub(lastPing) <= time.Minute {
				isOnline = true
			}
		}

		newState := &DeviceState{
			ID:       id,
			IsOnline: isOnline,
			LastPing: lastPing,
		}

		deviceCache.Store(macAddress, newState)
		deviceCache.Store(strings.ToUpper(macAddress), newState)
		count++
	}

	log.Printf("Presence service initialized: pre-loaded %d devices into memory cache\n", count)
}

func logWorker(workerID int) {
	defer wg.Done()
	for task := range logQueue {
		processStateLog(task)
	}
}

func enqueueLog(task LogTask) {
	closeMu.Lock()
	defer closeMu.Unlock()

	if isClosing {
		return
	}

	select {
	case logQueue <- task:
	default:
		log.Printf("Warning: logQueue full, dropping state log task for device %s\n", task.MacAddress)
	}
}

func processStateLog(task LogTask) {
	var lastState string
	logQuery := `SELECT state FROM "DeviceStatusLog" WHERE "deviceId" = $1 ORDER BY "createAt" DESC LIMIT 1`
	err := config.DB.QueryRow(logQuery, task.DeviceID).Scan(&lastState)

	expectedState := "OFFLINE"
	reason := "No sensor data received for > 1 minute"
	if task.IsOnline {
		expectedState = "ONLINE"
		reason = "Device sent sensor telemetry"
	}

	// Avoid duplicate state logs if current state in DB already matches
	if err == nil && lastState == expectedState {
		return
	}

	insertLog := `
	INSERT INTO "DeviceStatusLog" (id, "deviceId", state, reason, "createAt")
	VALUES (gen_random_uuid(), $1, $2, $3, NOW())
	`

	_, err = config.DB.Exec(insertLog, task.DeviceID, expectedState, reason)
	if err != nil {
		log.Printf("Failed to insert device status log for %s: %v\n", task.MacAddress, err)
		return
	}

	log.Printf("Presence state changed: %s is now %s\n", task.MacAddress, expectedState)
}

// MarkOnline marks a device as online upon receiving an MQTT packet
func MarkOnline(macAddress string) {
	now := time.Now()
	macAddress = strings.TrimSpace(macAddress)
	if macAddress == "" {
		return
	}

	val, exists := deviceCache.Load(macAddress)
	if !exists {
		val, exists = deviceCache.Load(strings.ToUpper(macAddress))
	}

	if !exists {
		var id string
		query := `SELECT id FROM "Device" WHERE "macAddress" = $1 OR UPPER("macAddress") = UPPER($1) LIMIT 1`
		err := config.DB.QueryRow(query, macAddress).Scan(&id)
		if err != nil {
			return
		}

		newState := &DeviceState{
			ID:       id,
			IsOnline: false,
			LastPing: now,
		}

		val, _ = deviceCache.LoadOrStore(macAddress, newState)
		deviceCache.Store(strings.ToUpper(macAddress), newState)
	}

	state := val.(*DeviceState)

	state.Mu.Lock()
	state.LastPing = now
	wasOffline := !state.IsOnline

	if wasOffline {
		state.IsOnline = true
	}
	state.Mu.Unlock()

	if wasOffline {
		enqueueLog(LogTask{
			DeviceID:   state.ID,
			MacAddress: macAddress,
			IsOnline:   true,
		})
	}
}

// SweepOfflineDevice iterates over cached devices and detects devices that have timed out
func SweepOfflineDevice() {
	now := time.Now()

	deviceCache.Range(func(key, value interface{}) bool {
		macAddress := key.(string)
		state := value.(*DeviceState)

		state.Mu.Lock()
		if state.IsOnline && now.Sub(state.LastPing) > time.Minute {
			state.IsOnline = false
			state.Mu.Unlock()

			enqueueLog(LogTask{
				DeviceID:   state.ID,
				MacAddress: macAddress,
				IsOnline:   false,
			})
		} else {
			state.Mu.Unlock()
		}

		return true
	})
}

// ClosePresenceService safely shuts down the log worker queue
func ClosePresenceService() {
	closeMu.Lock()
	if !isClosing {
		isClosing = true
		close(logQueue)
	}
	closeMu.Unlock()

	wg.Wait()
	log.Println("Presence service workers stopped cleanly")
}
