package handlers

import (
	// Стандартные библиотеки
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	// Внутренние пакеты
	"imagecleaner/internal/auth"
	"imagecleaner/internal/database"
	"imagecleaner/internal/services"

	// Сторонние библиотеки
	"github.com/gin-contrib/sessions" // Сессии все еще нужны для аутентификации
	"github.com/gin-gonic/gin"
)

// Константы для ограничений загрузки
const MaxUploadSize = 10 << 20 // 10 МБ
const MaxFiles = 10            // Максимальное количество файлов

// getEnv - локальная вспомогательная функция для получения переменных окружения.
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// ShowLoginPage отображает страницу входа.
// Больше не обрабатывает flash-сообщения.
func ShowLoginPage(c *gin.Context) {
	// Просто рендерим шаблон без дополнительных данных об ошибках/успехе.
	// Они будут переданы только при ответе на POST /login или после редиректа из HandleRegister.
	c.HTML(http.StatusOK, "login.html", gin.H{
		"title": "Вход",
		// "error":   nil, // Можно явно передать nil или не передавать вовсе
		// "success": nil,
	})
}

// ShowRegisterPage отображает страницу регистрации.
// Больше не обрабатывает flash-сообщения.
func ShowRegisterPage(c *gin.Context) {
	// Просто рендерим шаблон. Ошибки и username будут переданы при ответе на POST /register.
	c.HTML(http.StatusOK, "register.html", gin.H{
		"title": "Регистрация",
		// "error":      nil,
		// "success":    nil,
		// "username":   "",
	})
}

// HandleRegister обрабатывает POST-запрос с формы регистрации.
// При ошибках рендерит register.html с сообщением.
// При успехе рендерит login.html с сообщением об успехе.
func HandleRegister(c *gin.Context) {
	username := strings.TrimSpace(c.PostForm("username"))
	password := strings.TrimSpace(c.PostForm("password"))
	passwordConfirm := strings.TrimSpace(c.PostForm("password_confirm"))

	// Функция для рендеринга страницы регистрации с ошибкой
	renderRegisterWithError := func(message string) {
		c.HTML(http.StatusBadRequest, "register.html", gin.H{
			"title":    "Регистрация",
			"error":    message, // Передаем сообщение об ошибке
			"username": username,  // Передаем введенное имя пользователя обратно
		})
	}

	// Валидация
	if username == "" || password == "" || passwordConfirm == "" {
		renderRegisterWithError("Все поля должны быть заполнены")
		return
	}
	if len(password) < 8 {
		renderRegisterWithError("Пароль должен быть не менее 8 символов")
		return
	}
	if password != passwordConfirm {
		renderRegisterWithError("Пароли не совпадают")
		return
	}

	// Хеширование пароля
	hashedPassword, err := auth.HashPassword(password)
	if err != nil {
		log.Printf("Ошибка хеширования пароля для пользователя %s: %v", username, err)
		renderRegisterWithError("Произошла внутренняя ошибка при обработке пароля.")
		return
	}

	// Создание пользователя
	_, err = database.CreateUser(username, hashedPassword)
	if err != nil {
		log.Printf("Ошибка создания пользователя %s: %v", username, err)
		if strings.Contains(err.Error(), "уже существует") {
			renderRegisterWithError(err.Error()) // Показываем ошибку уникальности
		} else {
			renderRegisterWithError("Произошла внутренняя ошибка при создании пользователя.")
		}
		return
	}

	// Успешная регистрация - рендерим страницу ВХОДА с сообщением об успехе
	log.Printf("Пользователь %s успешно зарегистрирован.", username)
	c.HTML(http.StatusOK, "login.html", gin.H{
		"title":   "Вход",
		"success": "Вы успешно зарегистрированы! Теперь вы можете войти.", // Сообщение об успехе
	})
	// Редирект больше не нужен:
	// c.Redirect(http.StatusFound, "/login")
}

// HandleLogin обрабатывает POST-запрос с формы входа.
// При ошибках рендерит login.html с сообщением.
// При успехе сохраняет сессию и редиректит на /upload.
func HandleLogin(c *gin.Context) {
	username := strings.TrimSpace(c.PostForm("username"))
	password := strings.TrimSpace(c.PostForm("password"))

	// Функция для рендеринга страницы входа с ошибкой
	renderLoginWithError := func(message string) {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{ // Используем 401 для неверных данных
			"title": "Вход",
			"error": message, // Передаем сообщение об ошибке
			// Можно передать username обратно, если нужно предзаполнение при ошибке
			// "username": username,
		})
	}

	// Валидация
	if username == "" || password == "" {
		renderLoginWithError("Имя пользователя и пароль не могут быть пустыми")
		return
	}

	// Проверка пользователя
	user, err := database.GetUserByUsername(username)
	if err != nil {
		log.Printf("Ошибка получения пользователя %s из БД: %v", username, err)
		// При ошибке БД рендерим с общей ошибкой сервера
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{
			"title": "Вход",
			"error": "Ошибка сервера при проверке данных.",
		})
		return
	}

	// Проверка пароля
	if user == nil || !auth.CheckPasswordHash(password, user.PasswordHash) {
		log.Printf("Неудачная попытка входа для пользователя '%s'.", username)
		renderLoginWithError("Неверное имя пользователя или пароль.")
		return
	}

	// Успешный вход - сохраняем сессию и делаем редирект (здесь редирект оправдан)
	session := sessions.Default(c)
	session.Set("userID", user.ID)
	session.Set("username", user.Username)
	err = session.Save()
	if err != nil {
		log.Printf("Ошибка сохранения сессии после успешного входа пользователя %s (ID: %d): %v", username, user.ID, err)
		// Если сессию не сохранить, рендерим ошибку на странице входа
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{
			"title": "Вход",
			"error": "Не удалось сохранить данные сессии.",
		})
		return
	}

	log.Printf("Пользователь %s (ID: %d) успешно вошел в систему.", user.Username, user.ID)
	c.Redirect(http.StatusFound, "/upload") // Редирект на страницу загрузки
}

// ShowUploadPage отображает страницу загрузки (без flash).
func ShowUploadPage(c *gin.Context) {
	session := sessions.Default(c) // Нужна только для получения username
	username := session.Get("username")
	usernameStr, _ := username.(string)

	c.HTML(http.StatusOK, "upload.html", gin.H{
		"title":        "Загрузка изображения",
		"username":     usernameStr,
		"errors":       nil, // Нет ошибок при GET
		"success_urls": nil, // Нет URL при GET
	})
}

// HandleUpload обрабатывает загрузку и рендерит результат (без flash).
// (Код этой функции уже был исправлен в предыдущем шаге и не требует изменений здесь)
func HandleUpload(c *gin.Context) {
	// --- Предварительные проверки ---
	maxTotalSize := int64(MaxFiles*MaxUploadSize + 1*1024*1024)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxTotalSize)

	userID, exists := c.Get("userID")
	if !exists {
		log.Println("КРИТИЧЕСКАЯ ОШИБКА: userID не найден в контексте /upload.")
		c.Redirect(http.StatusFound, "/login")
		return
	}
	userID64 := userID.(int64)

	// Получаем имя пользователя для передачи в шаблон
	session := sessions.Default(c)
	username := session.Get("username")
	usernameStr, _ := username.(string)

	// --- Парсинг формы ---
	err := c.Request.ParseMultipartForm(int64(MaxUploadSize))
	if err != nil {
		log.Printf("Ошибка парсинга multipart формы для userID %d: %v", userID64, err)
		errorMsg := "Ошибка обработки запроса при загрузке файлов."
		if err.Error() == "http: request body too large" {
			errorMsg = fmt.Sprintf("Общий размер запроса слишком большой. Максимум около %d MB.", maxTotalSize/1024/1024)
		} else if strings.Contains(err.Error(), "multipart: NextPart") || strings.Contains(err.Error(), "unexpected EOF") || strings.Contains(err.Error(), "EOF") {
			errorMsg = fmt.Sprintf("Ошибка чтения данных файла. Возможно, один из файлов слишком большой (макс. %d MB на файл) или произошла ошибка передачи.", MaxUploadSize/1024/1024)
		}
		c.HTML(http.StatusBadRequest, "upload.html", gin.H{
			"title":        "Ошибка загрузки",
			"username":     usernameStr,
			"errors":       []string{errorMsg},
			"success_urls": nil,
		})
		return
	}

	files := c.Request.MultipartForm.File["imagefiles"]

	if len(files) == 0 {
		c.HTML(http.StatusBadRequest, "upload.html", gin.H{
			"title":        "Ошибка загрузки",
			"username":     usernameStr,
			"errors":       []string{"Вы не выбрали ни одного файла."},
			"success_urls": nil,
		})
		return
	}
	if len(files) > MaxFiles {
		c.HTML(http.StatusBadRequest, "upload.html", gin.H{
			"title":        "Ошибка загрузки",
			"username":     usernameStr,
			"errors":       []string{fmt.Sprintf("Можно загрузить не более %d файлов одновременно.", MaxFiles)},
			"success_urls": nil,
		})
		return
	}

	// --- Обработка каждого файла ---
	var successURLs []string
	var errorMessages []string
	uploadPath := getEnv("UPLOAD_PATH", "/app/uploads")
	baseURL := getEnv("BASE_URL", "")

	if baseURL == "" {
		log.Printf("КРИТИЧЕСКАЯ ОШИБКА КОНФИГУРАЦИИ: Переменная окружения BASE_URL не установлена!")
		errorMessages = append(errorMessages, "Ошибка конфигурации сервера: невозможно сгенерировать ссылки.")
	}

	for _, fileHeader := range files {
		log.Printf("Обработка файла: %s, Размер: %d байт (UserID: %d)",
			fileHeader.Filename, fileHeader.Size, userID64)

		if fileHeader.Size == 0 {
			errorMessages = append(errorMessages, fmt.Sprintf("Файл '%s': пустой и не будет обработан.", fileHeader.Filename))
			continue
		}
		if fileHeader.Size > MaxUploadSize {
			errorMessages = append(errorMessages, fmt.Sprintf("Файл '%s': слишком большой (%.2f MB > %.2f MB).", fileHeader.Filename, float64(fileHeader.Size)/1024/1024, float64(MaxUploadSize)/1024/1024))
			continue
		}

		storedFilename, errProc := services.ProcessAndSaveImage(fileHeader, uploadPath)
		if errProc != nil {
			log.Printf("Ошибка обработки/сохранения файла '%s' для userID %d: %v", fileHeader.Filename, userID64, errProc)
			errMsg := "Ошибка обработки файла."
			if strings.Contains(errProc.Error(), "Недопустимый тип файла") { errMsg = "Недопустимый тип файла (разрешены JPEG, PNG, GIF)." }
			if strings.Contains(errProc.Error(), "Не удалось декодировать") { errMsg = "Не удалось распознать формат файла или файл поврежден." }
			if strings.Contains(errProc.Error(), "не удалось создать файл") { errMsg = "Внутренняя ошибка сервера при сохранении файла." }
			errorMessages = append(errorMessages, fmt.Sprintf("Файл '%s': %s", fileHeader.Filename, errMsg))
			continue
		}

		accessToken, errToken := services.GenerateSecureToken(32)
		if errToken != nil {
			log.Printf("КРИТИЧЕСКАЯ ОШИБКА: не удалось сгенерировать токен для файла '%s' userID %d: %v", fileHeader.Filename, userID64, errToken)
			errorMessages = append(errorMessages, fmt.Sprintf("Файл '%s': Внутренняя ошибка сервера (токен).", fileHeader.Filename))
			cleanupFile(filepath.Join(uploadPath, storedFilename))
			continue
		}

		imageID, errDB := database.CreateImageRecord(userID64, fileHeader.Filename, storedFilename, accessToken)
		if errDB != nil {
			log.Printf("КРИТИЧЕСКАЯ ОШИБКА: не удалось сохранить запись в БД для файла '%s' userID %d: %v", fileHeader.Filename, userID64, errDB)
			errMsg := "Внутренняя ошибка сервера (БД)."
			if strings.Contains(errDB.Error(), "конфликт") { errMsg = "Внутренняя ошибка сервера (конфликт данных)." }
			errorMessages = append(errorMessages, fmt.Sprintf("Файл '%s': %s", fileHeader.Filename, errMsg))
			cleanupFile(filepath.Join(uploadPath, storedFilename))
			continue
		}

		if baseURL != "" {
			viewURL := fmt.Sprintf("%s/view/%s", baseURL, accessToken)
			successURLs = append(successURLs, viewURL)
			log.Printf("Файл '%s' (ID: %d) успешно обработан userID %d. URL: %s", fileHeader.Filename, imageID, userID64, viewURL)
		} else {
			log.Printf("Файл '%s' (ID: %d) успешно обработан userID %d, но URL не сформирован (BASE_URL не задан).", fileHeader.Filename, imageID, userID64)
			errorMessages = append(errorMessages, fmt.Sprintf("Файл '%s': успешно загружен, но ссылка не создана (ошибка конфигурации).", fileHeader.Filename))
		}
	} // Конец цикла for по файлам

	log.Printf("Завершена обработка %d файлов для userID %d. Успешно с URL: %d, Ошибки: %d.",
		len(files), userID64, len(successURLs), len(errorMessages))

	// --- ОТРИСОВКА РЕЗУЛЬТАТА ---
	c.HTML(http.StatusOK, "upload.html", gin.H{
		"title":        "Результаты загрузки",
		"username":     usernameStr,
		"errors":       errorMessages,
		"success_urls": successURLs,
	})
}

// HandleLogout использует редирект
func HandleLogout(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("userID")

	session.Delete("userID")
	session.Delete("username")
	session.Options(sessions.Options{MaxAge: -1})
	err := session.Save()
	if err != nil {
		log.Printf("Ошибка сохранения сессии после выхода пользователя (ID: %v): %v", userID, err)
	} else {
        log.Printf("Пользователь (ID: %v) успешно вышел из системы.", userID)
    }
	c.Redirect(http.StatusFound, "/")
}


// cleanupFile - вспомогательная функция для удаления файла по полному пути.
func cleanupFile(fullPath string) {
	if fullPath != "" {
		log.Printf("Попытка удаления файла %s из-за ошибки...", fullPath)
		err := os.Remove(fullPath)
		if err != nil {
			log.Printf("ПРЕДУПРЕЖДЕНИЕ: не удалось удалить файл %s после ошибки: %v", fullPath, err)
		} else {
			log.Printf("Файл %s успешно удален после ошибки обработки.", fullPath)
		}
	}
}

// ShowConfirmViewPage отображает страницу подтверждения просмотра.
func ShowConfirmViewPage(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"title": "Ошибка запроса", "message": "Отсутствует идентификатор изображения в ссылке."})
		return
	}

	img, err := database.GetImageByToken(token)
	if err != nil {
		log.Printf("Ошибка БД при поиске токена %s в ShowConfirmViewPage: %v", token, err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"title": "Ошибка сервера", "message": "Произошла ошибка при поиске информации об изображении."})
		return
	}

	if img == nil {
		log.Printf("Токен %s не найден в БД (GET /view).", token)
		c.HTML(http.StatusNotFound, "error.html", gin.H{"title": "Не найдено", "message": "Ссылка недействительна или устарела."})
		return
	}

	if img.Status != "pending" {
		log.Printf("Попытка доступа (GET /view) к уже использованному токену %s (статус: %s)", token, img.Status)
		c.HTML(http.StatusGone, "error.html", gin.H{"title": "Ссылка истекла", "message": "Эта ссылка уже была использована или срок её действия истёк."})
		return
	}

	c.HTML(http.StatusOK, "confirm_view.html", gin.H{
		"title": "Подтверждение просмотра",
		"token": token,
	})
}

// HandleConfirmView обрабатывает POST-запрос подтверждения просмотра.
func HandleConfirmView(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Отсутствует токен доступа в URL."})
		return
	}

	// 1. Повторно ищем изображение в БД
	img, err := database.GetImageByToken(token)
	if err != nil {
		log.Printf("Ошибка БД при повторном поиске токена %s (POST /view): %v", token, err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"title": "Ошибка сервера", "message": "Ошибка сервера при проверке ссылки."})
		c.Abort()
		return
	}

	// 2. Повторно проверяем статус
	if img == nil || img.Status != "pending" {
		log.Printf("Попытка повторного доступа (POST /view) или race condition для токена %s (статус: %s)", token, img.Status)
		c.HTML(http.StatusGone, "error.html", gin.H{"title": "Ссылка истекла", "message": "Ссылка недействительна или уже была использована."})
		c.Abort()
		return
	}

	// 3. Помечаем как просмотренное
	err = database.MarkImageViewed(token)
	if err != nil {
		log.Printf("Не удалось пометить токен %s как просмотренный (ImageID: %d): %v", token, img.ID, err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"title": "Ошибка сервера", "message": "Не удалось обработать ваш запрос на просмотр."})
		c.Abort()
		return
	}
	log.Printf("Токен %s успешно помечен как 'viewed' в БД (ImageID: %d) перед отправкой файла", token, img.ID)

	// 4. Отправляем файл
	uploadPath := getEnv("UPLOAD_PATH", "/app/uploads")
	filePath := filepath.Join(uploadPath, img.StoredFilename)

	// 4.1 Проверяем существование файла
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		log.Printf("КРИТИЧЕСКАЯ ОШИБКА: Файл %s не найден на диске для токена %s (ImageID: %d)!", filePath, token, img.ID)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"title": "Ошибка сервера", "message": "Ошибка: файл изображения не найден на сервере."})
		c.Abort()
		return
	} else if err != nil {
		log.Printf("КРИТИЧЕСКАЯ ОШИБКА: Ошибка доступа к файлу %s для токена %s (ImageID: %d): %v", filePath, token, img.ID, err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"title": "Ошибка сервера", "message": "Ошибка доступа к файлу изображения на сервере."})
		c.Abort()
		return
	}

	// 4.2 Устанавливаем заголовки кеширования
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	log.Printf("Отправка файла %s клиенту (Token: %s, ImageID: %d)", filePath, token, img.ID)
	// 4.3 Отправляем файл
	c.File(filePath)

	// 5. Запускаем удаление файла в горутине
	go func(pathToDelete string, imageID int64, tokenToDelete string) {
		time.Sleep(2 * time.Second) // Небольшая задержка

		log.Printf("Попытка асинхронного удаления файла %s для ImageID: %d (Token: %s) после просмотра", pathToDelete, imageID, tokenToDelete)
		err := os.Remove(pathToDelete)
		if err != nil {
			log.Printf("ОШИБКА АСИНХРОННОГО УДАЛЕНИЯ ФАЙЛА: не удалось удалить файл %s (ImageID: %d, Token: %s): %v", pathToDelete, imageID, tokenToDelete, err)
			// database.UpdateImageStatus(imageID, "delete_failed") // Опционально
		} else {
			log.Printf("Файл %s успешно удален асинхронно после просмотра (ImageID: %d, Token: %s).", pathToDelete, imageID, tokenToDelete)
			// database.UpdateImageStatus(imageID, "deleted") // Опционально
		}
	}(filePath, img.ID, token)
}