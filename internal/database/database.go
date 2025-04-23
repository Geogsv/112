package database

import (
	"database/sql" // Стандартная библиотека для работы с базами данных
	"fmt"
	"imagecleaner/internal/models"
	"log" // Стандартная библиотека для вывода логов в консоль
	"strings"
	"time"

	// Пустой импорт "_" используется для драйверов баз данных.
	// Мы импортируем пакет ради "побочного эффекта" - регистрации драйвера sqlite3
	// в пакете database/sql. Сами функции из пакета go-sqlite3 напрямую мы вызывать не будем.
	_ "github.com/mattn/go-sqlite3" // Библиотека для работы с базой данных SQLite3
)

// DB - это глобальная переменная для хранения объекта пула соединений с БД.
// Экспортируется (начинается с большой буквы), чтобы быть доступной из других пакетов,
// но лучше передавать его явно, где это возможно. Пока оставим так для простоты.

var DB *sql.DB


// InitDB инициализирует соединение с БД и создает таблицы, если их нет.
// Принимает имя файла БД (например, "./service.db")

func InitDB(dataSourceName string) error {
	var err error
	// sql.Open не устанавливает соединение сразу, а только подготавливает объект *sql.DB.
	// Первый аргумент - имя драйвера, который мы зарегистрировали через пустой импорт.
	DB, err = sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return fmt.Errorf("Ошибка при открытии %s: %w", dataSourceName, err)
	}

	// db.Ping() проверяет реальное соединение с базой данных.
	if err = DB.Ping(); err != nil {
		DB.Close()
			return fmt.Errorf("Ошибка при проверке соединения с %s: %w", dataSourceName, err)
	}

	log.Println("Успешно подключились к базе данных:", dataSourceName)

	// Создаем таблицы, если их нет
	err = createTables()
	if err != nil {
	DB.Close()
		return fmt.Errorf("Ошибка при создании таблиц: %w", err)
	}
	log.Println("Таблицы успешно проверены/созданы")
	return nil // Возвращаем nil, если все хорошо
}

func createTables() error {
	// Текст SQL-запроса для создания таблицы users.
	// Используем `IF NOT EXISTS`, чтобы запрос не выдавал ошибку, если таблица уже есть.
	usersTableSQL := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT, -- Уникальный ID пользователя, автоинкремент
		username TEXT NOT NULL UNIQUE,                -- Имя пользователя, должно быть уникальным
		password_hash TEXT NOT NULL                   -- Хеш пароля
	);`

	// Выполняем SQL-запрос для создания таблицы users
	_, err := DB.Exec(usersTableSQL)
	if err != nil {
		return fmt.Errorf("Ошибка при создании таблицы users: %w", err)
	}

	// Текст SQL-запроса для создания таблицы images.
	imagesTableSQL := `
	CREATE TABLE IF NOT EXISTS images (
		id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT, -- Уникальный ID картинки
		user_id INTEGER NOT NULL,                     -- ID пользователя, который загрузил картинку
		original_filename TEXT,                       -- Исходное имя файла (для информации)
		stored_filename TEXT NOT NULL UNIQUE,         -- Уникальное имя файла на сервере
		access_token TEXT NOT NULL UNIQUE,            -- Уникальный токен для доступа к ссылке
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP, -- Время создания записи (по умолчанию текущее)
		viewed_at DATETIME NULL,                      -- Время просмотра (NULL, если еще не просмотрено)
		status TEXT NOT NULL DEFAULT 'pending',       -- Статус ('pending', 'viewed', 'deleted')
		FOREIGN KEY(user_id) REFERENCES users(id)     -- Связь с таблицей users
	);`

	// Выполняем SQL-запрос для создания таблицы images
	_, err = DB.Exec(imagesTableSQL)
	if err != nil {
		return fmt.Errorf("Ошибка при создании таблицы images: %w", err)
	}

	return nil
}
// Возвращаем экземпляр базы данных, чтобы можно было использовать его в других частях кода.
func GetDB() *sql.DB {
	return DB
}


func CreateUser(username, passwordHash string) (int64, error) {
	// Подготавливаем SQL-запрос с плейсхолдерами (?)
	stms, err := DB.Prepare("INSERT INTO users (username, password_hash) VALUES (?, ?)")
	if err != nil {
		return 0, fmt.Errorf("Ошибка при подготовке запроса CreateUser: %w", err)
}

	// defer stmt.Close() гарантирует, что подготовленный запрос будет закрыт
	// при выходе из функции, даже если произойдет ошибка.
	defer stms.Close()

	// Выполняем подготовленный запрос, передавая реальные значения вместо плейсхолдеров.
	res, err := stms.Exec(username, passwordHash)
	if err != nil {
	// Обрабатываем возможную ошибку уникальности username
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, fmt.Errorf("Пользователь с таким именем '%s' уже существует", username)
		}
		return 0, fmt.Errorf("Ошибка при выполнении запроса CreateUser: %w", err)
	}


	// Получаем последний вставленный ID пользователя.
	lastID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("Ошибка при получении ID последнего пользователя CreateUser: %w", err)
	}

	log.Printf("Создан пользователь: %s (ID: %d)", username, lastID)
	return lastID, nil


}

func GetUserByUsername(username string) (*models.User, error) {
	//Создаем экземпляр структуры user, куда будем сохранить результат запроса.
	user := &models.User{}

	// QueryRow используется для запросов, которые гарантированно возвращают
	// не более одной строки используем плейсхолдер.
	row := DB.QueryRow("SELECT id, username, password_hash FROM users WHERE username = ?", username)

	// Сканируем результат запроса в поля структуры user.
	// Передаем указатели на поля (&user.ID, &user.Username, &user.PasswordHash),
	// чтобы Scan мог записать в них значения.
	err := row.Scan(&user.ID, &user.Username, &user.PasswordHash)

	if err != nil {
		// Проверяем специальную ошибку sql.ErrNoRows. Она означает, что запрос
		// выполнился успешно, но не нашел строк (пользователь не найден).
		// В этом случае мы не должны возвращать ошибку приложения, а просто nil-пользователя.
		if err == sql.ErrNoRows {
			return nil, nil // Пользователь ненайдет это не ошибка БД(НЕТ ДАННЫХ)
		}
		// Возвращаем ошибку приложения, если это не ошибка "нет данных"
		return nil, fmt.Errorf("ошибка сканирования результата GetUserByUsername для %s: %w", username, err)
	}

	return user, nil
}

// CreateImageRecord сохраняет информацию о загруженном изображении в БД.
func CreateImageRecord(userID int64, originalFilename, storedFilename, accessToken string) (int64, error) {
	stmt, err := DB.Prepare(`
		INSERT INTO images(user_id, original_filename, stored_filename, access_token, status)
		VALUES(?, ?, ?, ?, 'pending')
	`)
	if err != nil {
		return 0, fmt.Errorf("ошибка создания запроса CreateImageRecord: %w", err)
	}
	defer stmt.Close()

	res, err := stmt.Exec(userID, originalFilename, storedFilename, accessToken)
	if err != nil {
	
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return 0, fmt.Errorf("изображение с таким токеном уже существует")
	}
		return 0, fmt.Errorf("ошибка создания записи изображения: %w", err)
	}

	lastID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("ошибка получения ID записи изображения CreateImageRecord: %w", err)
	}
	log.Printf("Запись об изображении создана, ID: %d, Token: %s", lastID, accessToken)
	return lastID, nil
 
	}

	// ... InitDB, createTables, CreateUser, GetUserByUsername, CreateImageRecord ...

// GetImageByToken ищет запись об изображении по его уникальному токену доступа.
// Возвращает указатель на models.Image или nil, если не найдено.
func GetImageByToken(token string) (*models.Image, error) {
	img := &models.Image{}
	// Выбираем все нужные поля из таблицы images
	row := DB.QueryRow(`
		SELECT id, user_id, original_filename, stored_filename, access_token, created_at, viewed_at, status
		FROM images
		WHERE access_token = ?`, token)

	// Сканируем результат в поля структуры img. Обрати внимание на sql.NullTime для viewed_at.
	err := row.Scan(
		&img.ID,
		&img.UserID,
		&img.OriginalFilename,
		&img.StoredFilename,
		&img.AccessToken,
		&img.CreatedAt,
		&img.ViewedAt, // Сканируется в sql.NullTime
		&img.Status,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			// Запись не найдена - это не ошибка БД
			return nil, nil
		}
		// Другая ошибка БД
		return nil, fmt.Errorf("ошибка сканирования GetImageByToken для токена %s: %w", token, err)
	}

	// Запись найдена, возвращаем указатель на нее
	return img, nil
}

// MarkImageViewed обновляет статус изображения на 'viewed' и записывает время просмотра.
// Важно: обновляет только если текущий статус 'pending', чтобы избежать race conditions.
// Возвращает ошибку, если обновление не удалось или не было выполнено (например, статус был не 'pending').
func MarkImageViewed(token string) error {
	stmt, err := DB.Prepare(`
		UPDATE images
		SET status = 'viewed', viewed_at = ?
		WHERE access_token = ? AND status = 'pending'
	`)
	if err != nil {
		return fmt.Errorf("ошибка подготовки запроса MarkImageViewed: %w", err)
	}
	defer stmt.Close()

	// Выполняем запрос, передавая текущее время и токен
	res, err := stmt.Exec(time.Now(), token)
	if err != nil {
		return fmt.Errorf("ошибка выполнения запроса MarkImageViewed для токена %s: %w", token, err)
	}

	// Проверяем, была ли действительно обновлена строка.
	// Если status был не 'pending', то rowsAffected будет 0.
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		// Ошибка получения количества затронутых строк (маловероятно для SQLite)
		return fmt.Errorf("ошибка получения rowsAffected в MarkImageViewed: %w", err)
	}

	if rowsAffected == 0 {
		// Строка не была обновлена - возможно, статус уже был 'viewed' или 'deleted',
		// или токен невалидный (хотя его должны были проверить раньше).
		// Возвращаем ошибку, чтобы показать, что операция не удалась как ожидалось.
		return fmt.Errorf("изображение с токеном %s не найдено в статусе 'pending' для обновления", token)
	}

	log.Printf("Изображение с токеном %s помечено как 'viewed'.", token)
	return nil // Успех
}
