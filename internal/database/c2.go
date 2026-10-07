package database

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// C2ListenerRecord represents an active or configured ingress listener.
type C2ListenerRecord struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Protocol  string    `json:"protocol"`
	BindHost  string    `json:"bind_host"`
	BindPort  int       `json:"bind_port"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// C2BeaconRecord represents a registered target beacon.
type C2BeaconRecord struct {
	ID        string    `json:"id"`
	HostName  string    `json:"hostname"`
	IP        string    `json:"ip"`
	OS        string    `json:"os"`
	User      string    `json:"user"`
	PID       int       `json:"pid"`
	SleepSec  int       `json:"sleep_sec"`
	Status    string    `json:"status"`
	LastSeen  time.Time `json:"last_seen"`
	CreatedAt time.Time `json:"created_at"`
}

// C2TaskRecord represents a dispatched command task for a beacon.
type C2TaskRecord struct {
	ID        string    `json:"id"`
	BeaconID  string    `json:"beacon_id"`
	Type      string    `json:"type"`
	Payload   string    `json:"payload"`
	Status    string    `json:"status"`
	Output    string    `json:"output"`
	CreatedAt time.Time `json:"created_at"`
}

// ListC2Listeners returns all listeners.
func (db *DB) ListC2Listeners() ([]*C2ListenerRecord, error) {
	rows, err := db.Query(`SELECT id, name, protocol, bind_host, bind_port, status, created_at FROM c2_listeners ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing c2 listeners: %w", err)
	}
	defer rows.Close()

	var list []*C2ListenerRecord
	for rows.Next() {
		var r C2ListenerRecord
		if err := rows.Scan(&r.ID, &r.Name, &r.Protocol, &r.BindHost, &r.BindPort, &r.Status, &r.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, &r)
	}
	return list, nil
}

// CreateC2Listener persists a new listener.
func (db *DB) CreateC2Listener(r *C2ListenerRecord) (*C2ListenerRecord, error) {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	_, err := db.Exec(`INSERT INTO c2_listeners (id, name, protocol, bind_host, bind_port, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, r.Protocol, r.BindHost, r.BindPort, r.Status, r.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting c2 listener: %w", err)
	}
	return r, nil
}

// ListC2Beacons returns all registered beacons.
func (db *DB) ListC2Beacons() ([]*C2BeaconRecord, error) {
	rows, err := db.Query(`SELECT id, hostname, ip, os, user, pid, sleep_sec, status, last_seen, created_at FROM c2_beacons ORDER BY last_seen DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing c2 beacons: %w", err)
	}
	defer rows.Close()

	var list []*C2BeaconRecord
	for rows.Next() {
		var r C2BeaconRecord
		if err := rows.Scan(&r.ID, &r.HostName, &r.IP, &r.OS, &r.User, &r.PID, &r.SleepSec, &r.Status, &r.LastSeen, &r.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, &r)
	}
	return list, nil
}

// UpsertC2Beacon inserts or updates a beacon record.
func (db *DB) UpsertC2Beacon(r *C2BeaconRecord) (*C2BeaconRecord, error) {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	r.LastSeen = now

	_, err := db.Exec(`INSERT INTO c2_beacons (id, hostname, ip, os, user, pid, sleep_sec, status, last_seen, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			hostname=excluded.hostname,
			ip=excluded.ip,
			os=excluded.os,
			user=excluded.user,
			pid=excluded.pid,
			sleep_sec=excluded.sleep_sec,
			status=excluded.status,
			last_seen=excluded.last_seen`,
		r.ID, r.HostName, r.IP, r.OS, r.User, r.PID, r.SleepSec, r.Status, r.LastSeen, r.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("upserting c2 beacon: %w", err)
	}
	return r, nil
}

// CreateC2Task queues a task for a beacon.
func (db *DB) CreateC2Task(r *C2TaskRecord) (*C2TaskRecord, error) {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	_, err := db.Exec(`INSERT INTO c2_tasks (id, beacon_id, type, payload, status, output, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.BeaconID, r.Type, r.Payload, r.Status, r.Output, r.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting c2 task: %w", err)
	}
	return r, nil
}

// ListC2Tasks returns tasks for a beacon.
func (db *DB) ListC2Tasks(beaconID string) ([]*C2TaskRecord, error) {
	rows, err := db.Query(`SELECT id, beacon_id, type, payload, status, output, created_at FROM c2_tasks WHERE beacon_id = ? ORDER BY created_at DESC`, beaconID)
	if err != nil {
		return nil, fmt.Errorf("listing c2 tasks: %w", err)
	}
	defer rows.Close()

	var list []*C2TaskRecord
	for rows.Next() {
		var r C2TaskRecord
		if err := rows.Scan(&r.ID, &r.BeaconID, &r.Type, &r.Payload, &r.Status, &r.Output, &r.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, &r)
	}
	return list, nil
}
