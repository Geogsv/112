package models

import (
	"database/sql" // Нужен для типа sql.NullTime
	"time"         // Нужен для типа time.Time
)
// User представляет пользователя в системе.
// Поля структуры соответствуют столбцам в таблице 'users'.
// Имена полей начинаются с большой буквы, чтобы они были экспортируемыми.
type User struct {
	ID           int64  // Соответствует 'id INTEGER PRIMARY KEY'
	Username     string // Соответствует 'username TEXT UNIQUE NOT NULL'
	PasswordHash string // Соответствует 'password_hash TEXT NOT NULL'
	// Мы не храним пароль в чистом виде! Только хеш.
}

type Image struct {
	ID               int64        // 'id INTEGER PRIMARY KEY'
	UserID           int64        // 'user_id INTEGER NOT NULL' (ссылка на User.ID)
	OriginalFilename string       // 'original_filename TEXT'
	StoredFilename   string       // 'stored_filename TEXT UNIQUE NOT NULL'
	AccessToken      string       // 'access_token TEXT UNIQUE NOT NULL'
	CreatedAt        time.Time    // 'created_at DATETIME' - Go автоматически преобразует
	ViewedAt         sql.NullTime // 'viewed_at DATETIME NULL' - NullTime используется для полей, которые могут быть NULL в БД
	Status           string       // 'status TEXT' ('pending', 'viewed', 'deleted')
}
// Позже здесь можно добавить методы для этих структур, если понадобится.
// Например, метод для проверки статуса картинки: func (img *Image) IsViewed() bool { ... }