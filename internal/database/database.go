package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"proxy-convert/internal/logger"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

type Link struct {
	ID          int       `json:"id"`
	Tag         string    `json:"tag"`
	Source      string    `json:"source"`
	Link        string    `json:"link"`
	Status      int       `json:"status"`
	Fingerprint string    `json:"fingerprint"`
	Count       int       `json:"count"`
	CreateTime  time.Time `json:"create_time"`
	UpdateTime  time.Time `json:"update_time"`
}

func New(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := initDB(db); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return &DB{DB: db}, nil
}

func initDB(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS links (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tag VARCHAR(100),
			source VARCHAR(100),
			link TEXT NOT NULL UNIQUE,
			status INTEGER DEFAULT 0,
			fingerprint TEXT NOT NULL UNIQUE,
			count INTEGER DEFAULT 0,
			create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_link ON links(link)`,
		`CREATE INDEX IF NOT EXISTS idx_status ON links(status)`,
		`CREATE INDEX IF NOT EXISTS idx_fingerprint ON links(fingerprint)`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}

	if err := migrateDB(db); err != nil {
		return err
	}

	return nil
}

func migrateDB(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(links)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	hasCountColumn := false
	hasSourceColumn := false
	for rows.Next() {
		var cid int
		var name string
		var type_ string
		var notnull int
		var dfltValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &type_, &notnull, &dfltValue, &pk); err != nil {
			continue
		}
		if name == "count" {
			hasCountColumn = true
		}
		if name == "source" {
			hasSourceColumn = true
		}
	}

	if !hasCountColumn {
		_, err := db.Exec(`ALTER TABLE links ADD COLUMN count INTEGER DEFAULT 0 CHECK(count >= 0 AND count <= 20)`)
		if err != nil {
			return fmt.Errorf("failed to add count column: %w", err)
		}
		logger.Println("数据库迁移完成：已添加 count 字段")
	}

	if !hasSourceColumn {
		_, err := db.Exec(`ALTER TABLE links ADD COLUMN source VARCHAR(100) DEFAULT ''`)
		if err != nil {
			return fmt.Errorf("failed to add source column: %w", err)
		}
		logger.Println("数据库迁移完成：已添加 source 字段")
	}

	return nil
}

func (db *DB) AddLink(link string, status int, fingerprint, tag string) (int64, error) {
	return db.AddLinkWithSource(link, status, fingerprint, tag, "")
}

func (db *DB) AddLinkWithSource(link string, status int, fingerprint, tag, source string) (int64, error) {
	if fingerprint == "" {
		fingerprint = link
	}

	result, err := db.Exec(
		`INSERT INTO links (tag, source, link, status, fingerprint, count, create_time, update_time) 
		 VALUES (?, ?, ?, ?, ?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		tag, source, link, status, fingerprint,
	)
	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

func (db *DB) GetLink(id int) (*Link, error) {
	var link Link
	err := db.QueryRow(`SELECT id, tag, source, link, status, fingerprint, count, create_time, update_time FROM links WHERE id = ?`, id).Scan(
		&link.ID, &link.Tag, &link.Source, &link.Link, &link.Status, &link.Fingerprint,
		&link.Count, &link.CreateTime, &link.UpdateTime,
	)
	if err != nil {
		return nil, err
	}
	return &link, nil
}

func (db *DB) GetLinkByLink(link string) (*Link, error) {
	var l Link
	err := db.QueryRow(`SELECT id, tag, source, link, status, fingerprint, count, create_time, update_time FROM links WHERE link = ?`, link).Scan(
		&l.ID, &l.Tag, &l.Source, &l.Link, &l.Status, &l.Fingerprint,
		&l.Count, &l.CreateTime, &l.UpdateTime,
	)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (db *DB) GetAllLinks(statuses []int, limit, offset int) ([]Link, error) {
	var query string
	var args []interface{}

	if limit <= 0 {
		if len(statuses) == 0 {
			query = `SELECT id, tag, source, link, status, fingerprint, count, create_time, update_time FROM links ORDER BY id DESC`
			args = []interface{}{}
		} else {
			placeholders := strings.Repeat("?,", len(statuses))
			placeholders = placeholders[:len(placeholders)-1]
			query = fmt.Sprintf(`SELECT id, tag, source, link, status, fingerprint, count, create_time, update_time FROM links WHERE status IN (%s) ORDER BY id DESC`, placeholders)
			args = make([]interface{}, len(statuses))
			for i, s := range statuses {
				args[i] = s
			}
		}
	} else {
		if len(statuses) == 0 {
			query = `SELECT id, tag, source, link, status, fingerprint, count, create_time, update_time FROM links ORDER BY id DESC LIMIT ? OFFSET ?`
			args = []interface{}{limit, offset}
		} else {
			placeholders := strings.Repeat("?,", len(statuses))
			placeholders = placeholders[:len(placeholders)-1]
			query = fmt.Sprintf(`SELECT id, tag, source, link, status, fingerprint, count, create_time, update_time FROM links WHERE status IN (%s) ORDER BY id DESC LIMIT ? OFFSET ?`, placeholders)
			args = make([]interface{}, len(statuses)+2)
			for i, s := range statuses {
				args[i] = s
			}
			args[len(statuses)] = limit
			args[len(statuses)+1] = offset
		}
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []Link
	for rows.Next() {
		var link Link
		err := rows.Scan(
			&link.ID, &link.Tag, &link.Source, &link.Link, &link.Status, &link.Fingerprint,
			&link.Count, &link.CreateTime, &link.UpdateTime,
		)
		if err != nil {
			logger.Printf("Error scanning link: %v", err)
			continue
		}
		links = append(links, link)
	}

	return links, nil
}

func (db *DB) UpdateLink(id int, link *string, status *int, fingerprint *string) (bool, error) {
	updates := []string{}
	args := []interface{}{}

	if link != nil {
		updates = append(updates, "link = ?")
		args = append(args, *link)
	}
	if status != nil {
		updates = append(updates, "status = ?")
		args = append(args, *status)
	}
	if fingerprint != nil {
		updates = append(updates, "fingerprint = ?")
		args = append(args, *fingerprint)
	}

	if len(updates) == 0 {
		return false, nil
	}

	updates = append(updates, "update_time = CURRENT_TIMESTAMP")
	args = append(args, id)

	query := fmt.Sprintf("UPDATE links SET %s WHERE id = ?", strings.Join(updates, ", "))
	result, err := db.Exec(query, args...)
	if err != nil {
		return false, err
	}

	rowsAffected, _ := result.RowsAffected()
	return rowsAffected > 0, nil
}

func (db *DB) UpdateLinkStatusByLink(link string, status int) (bool, error) {
	result, err := db.Exec(
		`UPDATE links SET status = ?, update_time = CURRENT_TIMESTAMP WHERE link = ?`,
		status, link,
	)
	if err != nil {
		return false, err
	}

	rowsAffected, _ := result.RowsAffected()
	return rowsAffected > 0, nil
}

func (db *DB) UpdateLinkStatusAndCount(id int, status *int, count *int) (bool, error) {
	updates := []string{}
	args := []interface{}{}

	if status != nil {
		updates = append(updates, "status = ?")
		args = append(args, *status)
	}
	if count != nil {
		updates = append(updates, "count = ?")
		args = append(args, *count)
	}

	if len(updates) == 0 {
		return false, nil
	}

	updates = append(updates, "update_time = CURRENT_TIMESTAMP")
	args = append(args, id)

	query := fmt.Sprintf("UPDATE links SET %s WHERE id = ?", strings.Join(updates, ", "))
	result, err := db.Exec(query, args...)
	if err != nil {
		return false, err
	}

	rowsAffected, _ := result.RowsAffected()
	return rowsAffected > 0, nil
}

func (db *DB) DeleteLink(id int) (bool, error) {
	result, err := db.Exec(`DELETE FROM links WHERE id = ?`, id)
	if err != nil {
		return false, err
	}

	rowsAffected, _ := result.RowsAffected()
	return rowsAffected > 0, nil
}

func (db *DB) DeleteLinkByLink(link string) (bool, error) {
	result, err := db.Exec(`DELETE FROM links WHERE link = ?`, link)
	if err != nil {
		return false, err
	}

	rowsAffected, _ := result.RowsAffected()
	return rowsAffected > 0, nil
}

func (db *DB) CountLinks(statuses []int) (int, error) {
	var count int
	var err error

	if len(statuses) == 0 {
		err = db.QueryRow(`SELECT COUNT(*) FROM links`).Scan(&count)
	} else {
		placeholders := strings.Repeat("?,", len(statuses))
		placeholders = placeholders[:len(placeholders)-1]
		args := make([]interface{}, len(statuses))
		for i, s := range statuses {
			args[i] = s
		}
		err = db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM links WHERE status IN (%s)`, placeholders), args...).Scan(&count)
	}

	return count, err
}

func (db *DB) UpdateAllLinkStatus(oldStatus, newStatus int) (int64, error) {
	result, err := db.Exec(
		`UPDATE links SET status = ?, update_time = CURRENT_TIMESTAMP WHERE status = ?`,
		newStatus, oldStatus,
	)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

func (db *DB) DeleteOldUnavailableLinks() (int64, error) {
	result, err := db.Exec(
		`DELETE FROM links WHERE status = -1 AND create_time < datetime('now', '-6 months')`,
	)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}
